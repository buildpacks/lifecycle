package common

type Platform interface {
	API() string
	CodeFor(errType LifecycleExitError) int
	DecodeAnalyzedMetadata(path string) (AnalyzedMetadata, error)
	NewAnalyzedMetadataBuilder() AnalyzedMetadataBuilder
	SupportsAssetPackages() bool
	SupportsMixinValidation() bool
}
