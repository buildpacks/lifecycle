module github.com/buildpacks/lifecycle

require (
	github.com/BurntSushi/toml v1.1.0
	github.com/apex/log v1.9.0
	github.com/awslabs/amazon-ecr-credential-helper/ecr-login v0.0.0-20220324232016-7a06d24eebd7
	github.com/buildpacks/imgutil v0.0.0-20220310160537-4dd8bc60eaff
	github.com/chrismellard/docker-credential-acr-env v0.0.0-20220327082430-c57b701bfc08
	github.com/docker/docker v20.10.14+incompatible
	github.com/golang/mock v1.6.0
	github.com/google/go-cmp v0.5.7
	github.com/google/go-containerregistry v0.8.0
	github.com/heroku/color v0.0.6
	github.com/pkg/errors v0.9.1
	github.com/sclevine/spec v1.4.0
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20220325203850-36772127a21f
)

require (
	github.com/Azure/azure-sdk-for-go v46.4.0+incompatible // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/Azure/go-autorest v14.2.0+incompatible // indirect
	github.com/Azure/go-autorest/autorest v0.11.8 // indirect
	github.com/Azure/go-autorest/autorest/adal v0.9.5 // indirect
	github.com/Azure/go-autorest/autorest/azure/auth v0.5.2 // indirect
	github.com/Azure/go-autorest/autorest/azure/cli v0.4.1 // indirect
	github.com/Azure/go-autorest/autorest/date v0.3.0 // indirect
	github.com/Azure/go-autorest/logger v0.2.0 // indirect
	github.com/Azure/go-autorest/tracing v0.6.0 // indirect
	github.com/Microsoft/go-winio v0.5.1 // indirect
	github.com/aws/aws-sdk-go-v2 v1.7.1 // indirect
	github.com/aws/aws-sdk-go-v2/config v1.5.0 // indirect
	github.com/aws/aws-sdk-go-v2/credentials v1.3.1 // indirect
	github.com/aws/aws-sdk-go-v2/feature/ec2/imds v1.3.0 // indirect
	github.com/aws/aws-sdk-go-v2/internal/ini v1.1.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecr v1.4.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/ecrpublic v1.4.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/internal/presigned-url v1.2.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sso v1.3.1 // indirect
	github.com/aws/aws-sdk-go-v2/service/sts v1.6.0 // indirect
	github.com/aws/smithy-go v1.6.0 // indirect
	github.com/containerd/containerd v1.5.8 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.10.1 // indirect
	github.com/dimchansky/utfbom v1.1.0 // indirect
	github.com/docker/cli v20.10.12+incompatible // indirect
	github.com/docker/distribution v2.7.1+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.6.4 // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/form3tech-oss/jwt-go v3.2.2+incompatible // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/jmespath/go-jmespath v0.4.0 // indirect
	github.com/klauspost/compress v1.13.6 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6 // indirect
	github.com/morikuni/aec v1.0.0 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/vbatts/tar-split v0.11.2 // indirect
	golang.org/x/crypto v0.0.0-20210817164053-32db794688a5 // indirect
	golang.org/x/net v0.0.0-20211216030914-fe4d6282115f // indirect
	google.golang.org/genproto v0.0.0-20211208223120-3a66f561d7aa // indirect
	google.golang.org/grpc v1.43.0 // indirect
	google.golang.org/protobuf v1.27.1 // indirect
)

go 1.17

replace github.com/containerd/containerd => github.com/containerd/containerd v1.5.10
