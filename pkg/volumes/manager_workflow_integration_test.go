package volumes

import "testing"

func TestIntegrationLocalVolumeManagerWorkflow(t *testing.T) {
	t.Run("bind and named mounts", TestManagerResolveBindAndNamedVolumeMounts)
	t.Run("multiple targets", TestManagerResolveNamedVolumeMultipleTargetsNestedAndReadOnly)
	t.Run("project mapping precedence", TestManagerResolveProjectVolumeMappingTakesPrecedence)
	t.Run("warnings and missing volumes", TestManagerResolveMountsReportsWarningsAndMissingVolumes)
	t.Run("bind path resolution", TestBindResolverResolvesRelativeAbsoluteAndSymlinkDirectories)
	t.Run("list and prune", TestManagerListAndPruneVolumes)
	t.Run("create rollback", TestManagerCreateCleansManagedPathWhenStoreCreateFails)
	t.Run("driver removal failure", TestManagerRemoveKeepsStoreRecordWhenDriverRemoveFails)
	t.Run("active sandbox reference", TestManagerRemoveRejectsActiveSandboxVolumeReferences)
	t.Run("forced config reference removal", TestManagerRemoveForceSkipsConfigReferencesButNotSandboxReferences)
	t.Run("reference recheck", TestManagerRemoveForceBypassesStoreConfigReferenceRecheck)
	t.Run("sandbox pagination", TestManagerFindSandboxReferencesUsesPagination)
	t.Run("invalid bind sources", TestBindResolverRejectsMissingOrFileSource)
}
