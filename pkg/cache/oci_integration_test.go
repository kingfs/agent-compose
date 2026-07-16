package cache

import "testing"

func TestIntegrationOCICacheSharedLayerLifecycle(t *testing.T) {
	TestOCISourceRemovalPreservesSharedLayersAndRequiredMetadata(t)
}
