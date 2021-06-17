package platform

type Platform interface {
	API() string
	CodeFor(errType LifecycleExitError) int
	DecodeAnalyzedMetadataFile(path string) (AnalyzedMetadata, error)
	NewAnalyzedMetadata(config AnalyzedMetadataConfig) AnalyzedMetadata
	SupportsAssetPackages() bool
	SupportsMixinValidation() bool
}
