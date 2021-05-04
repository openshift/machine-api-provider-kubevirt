module github.com/openshift/cluster-api-provider-kubevirt

go 1.13

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/golang/mock v1.4.4
	github.com/openshift/machine-api-operator v0.2.1-0.20210505133115-b7ef098180db
	github.com/pkg/errors v0.9.1
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.21.0
	k8s.io/apimachinery v0.21.0
	k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
	kubevirt.io/client-go v0.0.0-00010101000000-000000000000
	kubevirt.io/containerized-data-importer v1.10.6
	sigs.k8s.io/controller-runtime v0.9.0-beta.1.0.20210512131817-ce2f0c92d77e
	sigs.k8s.io/controller-tools v0.4.1
	sigs.k8s.io/yaml v1.2.0
)

replace (
	k8s.io/api => k8s.io/api v0.21.0
	k8s.io/client-go => k8s.io/client-go v0.21.0
	kubevirt.io/client-go => kubevirt.io/client-go v0.29.0
	kubevirt.io/containerized-data-importer => kubevirt.io/containerized-data-importer v1.10.6
	sigs.k8s.io/cluster-api-provider-aws => github.com/openshift/cluster-api-provider-aws v0.2.1-0.20210505150511-f9cb840ae412
	sigs.k8s.io/cluster-api-provider-azure => github.com/openshift/cluster-api-provider-azure v0.1.0-alpha.3.0.20210505133115-b2eda16dd665
)
