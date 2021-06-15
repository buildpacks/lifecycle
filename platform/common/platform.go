package common

type Platform interface {
	API() string
	CodeFor(errType LifecycleExitError) int
	DecodeAnalyzedMetadataFile(path string) (AnalyzedMetadata, error)
	NewAnalyzedMetadataBuilder() AnalyzedMetadataBuilder
	SupportsAssetPackages() bool
	SupportsMixinValidation() bool
}
