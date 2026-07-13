package resources

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"agent-compose/pkg/identity"
	"agent-compose/pkg/images"
	domain "agent-compose/pkg/model"
	"agent-compose/pkg/runtimecache"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

type StoredSource interface {
	ResolveStoredResources(context.Context, domain.ResourceResolveOptions) ([]domain.ResolvedResource, error)
}

type SandboxSource interface {
	GetSandbox(context.Context, string) (*domain.Sandbox, error)
	ListSandboxes(context.Context, domain.SandboxListOptions) (domain.SandboxListResult, error)
}

type CacheSource interface {
	ListCaches(context.Context, runtimecache.ListRequest) (runtimecache.ListResult, error)
}

type Resolver struct {
	stored    StoredSource
	sandboxes SandboxSource
	images    images.Backend
	caches    CacheSource
}

func NewResolver(stored StoredSource, sandboxes SandboxSource, imageBackend images.Backend, caches CacheSource) *Resolver {
	return &Resolver{stored: stored, sandboxes: sandboxes, images: imageBackend, caches: caches}
}

func (r *Resolver) Resolve(ctx context.Context, options domain.ResourceResolveOptions) (domain.ResourceResolveResult, error) {
	options.Ref = strings.TrimSpace(options.Ref)
	if options.Ref == "" {
		return domain.ResourceResolveResult{}, fmt.Errorf("resource ref is required")
	}
	allowed, err := normalizeKinds(options.Kinds)
	if err != nil {
		return domain.ResourceResolveResult{}, err
	}
	options.Kinds = kindList(allowed)

	result := domain.ResourceResolveResult{}
	if r == nil || r.stored == nil {
		return result, fmt.Errorf("stored resource resolver is required")
	}
	stored, err := r.stored.ResolveStoredResources(ctx, options)
	if err != nil {
		return result, fmt.Errorf("resolve stored resources: %w", err)
	}
	result.Resources = append(result.Resources, stored...)

	type sourceResult struct {
		resources []domain.ResolvedResource
		warnings  []string
	}
	var tasks []func() sourceResult
	hasExactName := hasMatchType(stored, domain.ResourceMatchName)
	if !hasExactName && allows(allowed, domain.ResourceKindSandbox) && identity.IsIDPrefix(options.Ref) {
		tasks = append(tasks, func() sourceResult {
			matches, warning := r.resolveSandboxes(ctx, options.Ref)
			return sourceResult{resources: matches, warnings: appendWarning(nil, warning)}
		})
	}
	if !hasExactName && allows(allowed, domain.ResourceKindCache) && identity.IsIDPrefix(options.Ref) {
		tasks = append(tasks, func() sourceResult {
			matches, warning := r.resolveCaches(ctx, options.Ref)
			return sourceResult{resources: matches, warnings: appendWarning(nil, warning)}
		})
	}
	if allows(allowed, domain.ResourceKindImage) {
		tasks = append(tasks, func() sourceResult {
			matches, warnings := r.resolveImages(ctx, options.Ref)
			return sourceResult{resources: matches, warnings: warnings}
		})
	}
	results := make(chan sourceResult, len(tasks))
	for _, task := range tasks {
		go func() { results <- task() }()
	}
	for range tasks {
		part := <-results
		result.Resources = append(result.Resources, part.resources...)
		result.Warnings = append(result.Warnings, part.warnings...)
	}

	result.Resources = bestUniqueMatches(result.Resources)
	result.Warnings = uniqueSortedStrings(result.Warnings)
	return result, nil
}

func hasMatchType(resources []domain.ResolvedResource, matchType domain.ResourceMatchType) bool {
	for _, resource := range resources {
		if resource.MatchType == matchType {
			return true
		}
	}
	return false
}

func (r *Resolver) resolveSandboxes(ctx context.Context, ref string) ([]domain.ResolvedResource, string) {
	if r.sandboxes == nil {
		return nil, "sandbox resolver is unavailable"
	}
	if isFullIDRef(ref) {
		sandbox, err := r.sandboxes.GetSandbox(ctx, ref)
		if err == nil && sandbox != nil {
			return []domain.ResolvedResource{sandboxResource(sandbox, domain.ResourceMatchID)}, ""
		}
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Sprintf("resolve sandbox %q: %v", ref, err)
		}
		return nil, ""
	}
	listed, err := r.sandboxes.ListSandboxes(ctx, domain.SandboxListOptions{Limit: int(^uint(0) >> 1)})
	if err != nil {
		return nil, fmt.Sprintf("resolve sandbox %q: %v", ref, err)
	}
	matches := make([]domain.ResolvedResource, 0)
	for _, sandbox := range listed.Sandboxes {
		if sandbox == nil || !IDMatchesRef(sandbox.Summary.ID, ref) {
			continue
		}
		matches = append(matches, sandboxResource(sandbox, resourceIDMatchType(sandbox.Summary.ID, ref)))
	}
	return matches, ""
}

func sandboxResource(sandbox *domain.Sandbox, matchType domain.ResourceMatchType) domain.ResolvedResource {
	id := strings.TrimSpace(sandbox.Summary.ID)
	shortID := strings.TrimSpace(sandbox.Summary.ShortID)
	if shortID == "" {
		shortID = shortIDFor(id)
	}
	return domain.ResolvedResource{
		Kind:       domain.ResourceKindSandbox,
		MatchType:  matchType,
		ID:         id,
		ShortID:    shortID,
		InspectRef: id,
	}
}

func (r *Resolver) resolveCaches(ctx context.Context, ref string) ([]domain.ResolvedResource, string) {
	if r.caches == nil {
		return nil, "cache resolver is unavailable"
	}
	listed, err := r.caches.ListCaches(ctx, runtimecache.ListRequest{})
	if err != nil {
		return nil, fmt.Sprintf("resolve cache %q: %v", ref, err)
	}
	matches := make([]domain.ResolvedResource, 0)
	for _, item := range listed.Items {
		if !IDMatchesRef(item.CacheID, ref) {
			continue
		}
		matches = append(matches, domain.ResolvedResource{
			Kind:       domain.ResourceKindCache,
			MatchType:  resourceIDMatchType(item.CacheID, ref),
			ID:         item.CacheID,
			ShortID:    shortIDFor(item.CacheID),
			InspectRef: item.CacheID,
		})
	}
	return matches, ""
}

func (r *Resolver) resolveImages(ctx context.Context, ref string) ([]domain.ResolvedResource, []string) {
	if r.images == nil {
		return nil, []string{"image resolver is unavailable"}
	}
	var matches []domain.ResolvedResource
	var warnings []string

	inspected, err := r.images.InspectImage(ctx, images.InspectRequest{ImageRef: ref})
	if err == nil && inspected.Image != nil {
		if matchType := imageMatchType(inspected.Image, ref, true); matchType != "" {
			matches = append(matches, imageResource(inspected.Image, ref, matchType))
		}
	} else if err != nil && !images.IsNotFound(err) && !isInvalidImageReference(err) {
		warnings = append(warnings, fmt.Sprintf("resolve image %q: %v", ref, err))
	}

	if identity.IsIDPrefix(ref) && !isFullIDRef(ref) {
		listed, listErr := r.images.ListImages(ctx, images.ListRequest{All: true})
		if listErr != nil {
			if err == nil || listErr.Error() != err.Error() {
				warnings = append(warnings, fmt.Sprintf("resolve image id %q: %v", ref, listErr))
			}
		} else {
			for _, image := range listed.Images {
				matchType := imageMatchType(image, ref, false)
				if matchType == "" {
					continue
				}
				inspectRef := strings.TrimSpace(image.GetImageId())
				if matchType == domain.ResourceMatchName {
					inspectRef = ref
				}
				matches = append(matches, imageResource(image, inspectRef, matchType))
			}
		}
	}
	return matches, warnings
}

func imageResource(image *agentcomposev2.Image, inspectRef string, matchType domain.ResourceMatchType) domain.ResolvedResource {
	id := strings.TrimSpace(image.GetImageId())
	name := firstNonEmptyImageValue(image.GetImageRef(), firstImageValue(image.GetRepoTags()), image.GetResolvedRef(), firstImageValue(image.GetRepoDigests()))
	return domain.ResolvedResource{
		Kind:       domain.ResourceKindImage,
		MatchType:  matchType,
		ID:         id,
		ShortID:    shortIDFor(id),
		Name:       name,
		InspectRef: strings.TrimSpace(inspectRef),
	}
}

func imageMatchType(image *agentcomposev2.Image, ref string, inspected bool) domain.ResourceMatchType {
	if image == nil {
		return ""
	}
	ref = strings.TrimSpace(ref)
	if !identity.IsIDPrefix(ref) {
		for _, alias := range imageAliases(image) {
			if strings.EqualFold(strings.TrimSpace(alias), ref) {
				return domain.ResourceMatchName
			}
		}
	}
	if IDMatchesRef(image.GetImageId(), ref) {
		return resourceIDMatchType(image.GetImageId(), ref)
	}
	if inspected {
		// A successful backend inspection is authoritative for native shorthand
		// references such as "ubuntu", even if the list representation only
		// exposes the normalized "ubuntu:latest" tag.
		return domain.ResourceMatchName
	}
	return ""
}

func imageAliases(image *agentcomposev2.Image) []string {
	aliases := make([]string, 0, 3+len(image.GetRepoTags())+len(image.GetRepoDigests()))
	aliases = append(aliases, image.GetImageRef(), image.GetResolvedRef())
	aliases = append(aliases, image.GetRepoTags()...)
	aliases = append(aliases, image.GetRepoDigests()...)
	return aliases
}

func firstImageValue(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func firstNonEmptyImageValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func normalizeKinds(kinds []domain.ResourceKind) (map[domain.ResourceKind]bool, error) {
	allowed := make(map[domain.ResourceKind]bool)
	for _, kind := range kinds {
		switch kind {
		case domain.ResourceKindProject, domain.ResourceKindAgent, domain.ResourceKindRun, domain.ResourceKindSandbox,
			domain.ResourceKindImage, domain.ResourceKindCache, domain.ResourceKindVolume:
			allowed[kind] = true
		default:
			return nil, fmt.Errorf("unsupported resource kind %q", kind)
		}
	}
	return allowed, nil
}

func kindList(allowed map[domain.ResourceKind]bool) []domain.ResourceKind {
	if len(allowed) == 0 {
		return nil
	}
	result := make([]domain.ResourceKind, 0, len(allowed))
	for _, kind := range allKinds() {
		if allowed[kind] {
			result = append(result, kind)
		}
	}
	return result
}

func allows(allowed map[domain.ResourceKind]bool, kind domain.ResourceKind) bool {
	return len(allowed) == 0 || allowed[kind]
}

func allKinds() []domain.ResourceKind {
	return []domain.ResourceKind{
		domain.ResourceKindProject,
		domain.ResourceKindAgent,
		domain.ResourceKindRun,
		domain.ResourceKindSandbox,
		domain.ResourceKindImage,
		domain.ResourceKindCache,
		domain.ResourceKindVolume,
	}
}

func IDMatchesRef(id, ref string) bool {
	idHash, err := identity.Hash(id)
	if err != nil || !identity.IsIDPrefix(ref) {
		return false
	}
	ref = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(ref)), identity.Prefix)
	return strings.HasPrefix(idHash, ref)
}

func isFullIDRef(ref string) bool {
	_, err := identity.Hash(ref)
	return err == nil
}

func resourceIDMatchType(id, ref string) domain.ResourceMatchType {
	idHash, idErr := identity.Hash(id)
	refHash, refErr := identity.Hash(ref)
	if idErr == nil && refErr == nil && idHash == refHash {
		return domain.ResourceMatchID
	}
	return domain.ResourceMatchIDPrefix
}

func shortIDFor(id string) string {
	if shortID := identity.ShortID(id); shortID != "" {
		return shortID
	}
	id = strings.TrimPrefix(strings.TrimSpace(id), identity.Prefix)
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func bestUniqueMatches(matches []domain.ResolvedResource) []domain.ResolvedResource {
	unique := make(map[string]domain.ResolvedResource)
	for _, match := range matches {
		key := resourceKey(match)
		if existing, ok := unique[key]; !ok || matchRank(match.MatchType) > matchRank(existing.MatchType) {
			unique[key] = match
		}
	}
	bestRank := 0
	for _, match := range unique {
		if rank := matchRank(match.MatchType); rank > bestRank {
			bestRank = rank
		}
	}
	result := make([]domain.ResolvedResource, 0, len(unique))
	for _, match := range unique {
		if matchRank(match.MatchType) == bestRank {
			result = append(result, match)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		left := fmt.Sprintf("%02d\x00%s\x00%s\x00%s", kindRank(result[i].Kind), result[i].ProjectName, result[i].Name, result[i].ID)
		right := fmt.Sprintf("%02d\x00%s\x00%s\x00%s", kindRank(result[j].Kind), result[j].ProjectName, result[j].Name, result[j].ID)
		return left < right
	})
	return result
}

func resourceKey(match domain.ResolvedResource) string {
	id := strings.TrimSpace(match.ID)
	if hash, err := identity.Hash(id); err == nil {
		id = hash
	}
	if id != "" {
		return strings.Join([]string{string(match.Kind), match.ProjectID, id}, "\x00")
	}
	return strings.Join([]string{string(match.Kind), match.ProjectID, match.Name, match.InspectRef}, "\x00")
}

func matchRank(matchType domain.ResourceMatchType) int {
	switch matchType {
	case domain.ResourceMatchName:
		return 3
	case domain.ResourceMatchID:
		return 2
	case domain.ResourceMatchIDPrefix:
		return 1
	default:
		return 0
	}
}

func kindRank(kind domain.ResourceKind) int {
	for index, candidate := range allKinds() {
		if candidate == kind {
			return index
		}
	}
	return len(allKinds())
}

func appendWarning(warnings []string, warning string) []string {
	if strings.TrimSpace(warning) == "" {
		return warnings
	}
	return append(warnings, warning)
}

func uniqueSortedStrings(values []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func isInvalidImageReference(err error) bool {
	_, kind, ok := images.ClassifyBackendError(err)
	return ok && kind == images.ErrorKindInvalidReference
}
