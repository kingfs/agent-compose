package agentcompose

import (
	"context"
	"database/sql"
	"errors"
	"time"

	loaderpkg "agent-compose/pkg/agentcompose/loader"
)

func (s *ConfigStore) loaderStore() *loaderpkg.Store {
	return loaderpkg.NewStore(s.db)
}

func (s *ConfigStore) ensureLoaderSchema(ctx context.Context) error {
	return s.loaderStore().EnsureSchema(ctx)
}

func normalizeLoader(item Loader, assignID bool) (Loader, error) {
	return loaderpkg.Normalize(item, assignID)
}

func (s *ConfigStore) getLoaderIfExists(ctx context.Context, loaderID string) (Loader, bool, error) {
	item, err := s.GetLoader(ctx, loaderID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Loader{}, false, nil
		}
		return Loader{}, false, err
	}
	return item, true, nil
}

func normalizeLoaderTrigger(loaderID string, trigger LoaderTrigger) (LoaderTrigger, error) {
	return loaderpkg.NormalizeTrigger(loaderID, trigger)
}

func encodeCapsetIDs(ids []string) (string, error) {
	return loaderpkg.EncodeCapsetIDs(ids)
}

func decodeCapsetIDs(raw string) []string {
	return loaderpkg.DecodeCapsetIDs(raw)
}

func ensureColumn(ctx context.Context, db *sql.DB, table, column, definition string) error {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN "+column+" "+definition)
	return err
}

func (s *ConfigStore) CreateLoader(ctx context.Context, item Loader) (Loader, error) {
	return s.loaderStore().CreateLoader(ctx, item)
}

func (s *ConfigStore) UpdateLoader(ctx context.Context, item Loader) (Loader, error) {
	return s.loaderStore().UpdateLoader(ctx, item)
}

func (s *ConfigStore) UpsertManagedLoader(ctx context.Context, item Loader) (Loader, error) {
	return s.loaderStore().UpsertManagedLoader(ctx, item)
}

func (s *ConfigStore) DeleteLoader(ctx context.Context, loaderID string) error {
	return s.loaderStore().DeleteLoader(ctx, loaderID)
}

func (s *ConfigStore) DisableLoadersByDefaultAgent(ctx context.Context, agentID string) (int, error) {
	return s.loaderStore().DisableLoadersByDefaultAgent(ctx, agentID)
}

func (s *ConfigStore) ListLoaderSummaries(ctx context.Context) ([]LoaderSummary, error) {
	return s.loaderStore().ListLoaderSummaries(ctx)
}

func (s *ConfigStore) GetLoader(ctx context.Context, loaderID string) (Loader, error) {
	return s.loaderStore().GetLoader(ctx, loaderID)
}

func (s *ConfigStore) ListLoaders(ctx context.Context) ([]Loader, error) {
	return s.loaderStore().ListLoaders(ctx)
}

func (s *ConfigStore) ListManagedLoaders(ctx context.Context, projectID string) ([]Loader, error) {
	return s.loaderStore().ListManagedLoaders(ctx, projectID)
}

func (s *ConfigStore) ReplaceLoaderTriggers(ctx context.Context, loaderID string, triggers []LoaderTrigger) ([]LoaderTrigger, error) {
	return s.loaderStore().ReplaceLoaderTriggers(ctx, loaderID, triggers)
}

func (s *ConfigStore) SetLoaderEnabled(ctx context.Context, loaderID string, enabled bool) error {
	return s.loaderStore().SetLoaderEnabled(ctx, loaderID, enabled)
}

func (s *ConfigStore) SetLoaderTriggerEnabled(ctx context.Context, loaderID, triggerID string, enabled bool) error {
	return s.loaderStore().SetLoaderTriggerEnabled(ctx, loaderID, triggerID, enabled)
}

func (s *ConfigStore) UpdateLoaderLastError(ctx context.Context, loaderID, lastError string) error {
	return s.loaderStore().UpdateLoaderLastError(ctx, loaderID, lastError)
}

func (s *ConfigStore) MarkLoaderTriggerFired(ctx context.Context, loaderID, triggerID string, lastFiredAt, nextFireAt time.Time) error {
	return s.loaderStore().MarkLoaderTriggerFired(ctx, loaderID, triggerID, lastFiredAt, nextFireAt)
}

func (s *ConfigStore) CreateLoaderRun(ctx context.Context, run LoaderRunSummary) error {
	return s.loaderStore().CreateLoaderRun(ctx, run)
}

func (s *ConfigStore) UpdateLoaderRun(ctx context.Context, run LoaderRunSummary) error {
	return s.loaderStore().UpdateLoaderRun(ctx, run)
}

func (s *ConfigStore) GetLoaderRun(ctx context.Context, loaderID, runID string) (LoaderRunSummary, error) {
	return s.loaderStore().GetLoaderRun(ctx, loaderID, runID)
}

func (s *ConfigStore) ListLoaderRuns(ctx context.Context, loaderID string, limit int) ([]LoaderRunSummary, error) {
	return s.loaderStore().ListLoaderRuns(ctx, loaderID, limit)
}

func (s *ConfigStore) ListRecentLoaderRuns(ctx context.Context, limit int) ([]LoaderRunSummary, error) {
	return s.loaderStore().ListRecentLoaderRuns(ctx, limit)
}

func (s *ConfigStore) AddLoaderEvent(ctx context.Context, event LoaderEvent) error {
	return s.loaderStore().AddLoaderEvent(ctx, event)
}

func (s *ConfigStore) ListLoaderEvents(ctx context.Context, loaderID string, limit int) ([]LoaderEvent, error) {
	return s.loaderStore().ListLoaderEvents(ctx, loaderID, limit)
}

func (s *ConfigStore) GetLoaderState(ctx context.Context, loaderID, key string) (string, bool, error) {
	return s.loaderStore().GetLoaderState(ctx, loaderID, key)
}

func (s *ConfigStore) SetLoaderState(ctx context.Context, loaderID, key, valueJSON string) error {
	return s.loaderStore().SetLoaderState(ctx, loaderID, key, valueJSON)
}

func (s *ConfigStore) DeleteLoaderState(ctx context.Context, loaderID, key string) error {
	return s.loaderStore().DeleteLoaderState(ctx, loaderID, key)
}

func (s *ConfigStore) GetLoaderBinding(ctx context.Context, loaderID string) (LoaderBinding, bool, error) {
	return s.loaderStore().GetLoaderBinding(ctx, loaderID)
}

func (s *ConfigStore) UpsertLoaderBinding(ctx context.Context, binding LoaderBinding) error {
	return s.loaderStore().UpsertLoaderBinding(ctx, binding)
}
