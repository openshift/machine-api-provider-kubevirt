module github.com/openshift/cluster-api-provider-kubevirt

go 1.13

require (
	github.com/blang/semver v3.5.1+incompatible
	github.com/gogo/protobuf v1.3.1 // indirect
	github.com/golang/mock v1.2.0
	github.com/google/gofuzz v1.1.0 // indirect
	github.com/openshift/machine-api-operator v0.2.1-0.20201205203935-bf4ec1b2e645
	golang.org/x/crypto v0.0.0-20191205180655-e7c4368fe9dd // indirect
	gotest.tools v2.2.0+incompatible
	k8s.io/api v0.18.3
	k8s.io/apiextensions-apiserver v0.18.0-rc.1 // indirect
	k8s.io/apimachinery v0.18.3
	k8s.io/client-go v12.0.0+incompatible
	// k8s.io/client-go v12.0.0+incompatible
	k8s.io/klog v1.0.0
	k8s.io/kube-openapi v0.0.0-20200410145947-61e04a5be9a6 // indirect
	kubevirt.io/client-go v0.29.0
	kubevirt.io/containerized-data-importer v1.10.9
	sigs.k8s.io/controller-runtime v0.6.0
	sigs.k8s.io/controller-tools v0.2.4
	sigs.k8s.io/yaml v1.2.0
)

replace (
	github.com/appscode/jsonpatch => github.com/appscode/jsonpatch v1.0.1
	github.com/coreos/prometheus-operator => github.com/coreos/prometheus-operator v0.35.0
	github.com/go-kit/kit => github.com/go-kit/kit v0.3.0
	github.com/googleapis/gnostic => github.com/googleapis/gnostic v0.3.1
	// github.com/openshift/machine-api-operator => github.com/openshift/machine-api-operator v0.2.1-0.20191025120018-fb3724fc7bdf
	github.com/prometheus/prometheus => github.com/prometheus/prometheus v0.0.0-20190424153033-d3245f150225
	k8s.io/api => k8s.io/api v0.16.4
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.16.4
	k8s.io/apimachinery => k8s.io/apimachinery v0.16.4
	k8s.io/apiserver => k8s.io/apiserver v0.16.4
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.16.4
	k8s.io/client-go => k8s.io/client-go v0.16.4
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.16.4
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.16.4
	k8s.io/code-generator => k8s.io/code-generator v0.16.4
	k8s.io/component-base => k8s.io/component-base v0.16.4
	k8s.io/cri-api => k8s.io/cri-api v0.16.4
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.16.4
	k8s.io/klog => k8s.io/klog v0.4.0
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.16.4
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.16.4
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20191107075043-30be4d16710a
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.16.4
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.16.4
	// k8s.io/kubectl => k8s.io/kubectl v0.16.4
	k8s.io/kubectl => k8s.io/kubectl v0.0.0-20200124035537-9f7d91504e51
	k8s.io/kubelet => k8s.io/kubelet v0.16.4
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.16.4
	k8s.io/metrics => k8s.io/metrics v0.16.4
	k8s.io/node-api => k8s.io/node-api v0.16.4
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.16.4
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.16.4
	k8s.io/sample-controller => k8s.io/sample-controller v0.16.4
	kubevirt.io/client-go => kubevirt.io/client-go v0.29.2
)

// sigs.k8s.io/controller-runtime 0.6.0
// and
// github.com/openshift/machine-api-operator v0.2.1-0.20200402110321-4f3602b96da3 (throgh its import github.com/openshift/client-go)
// requires k8s-* v0.18.2
// but we are pinned to kubernetes-1.16.4 as for kubevirt.io/kubevirt v0.29.1 while we explicitly
// need github.com/operator-framework/api v0.3.5
replace (
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20200116152001-92a2713fa240
	github.com/operator-framework/api => github.com/operator-framework/api v0.3.5
	sigs.k8s.io/controller-runtime => sigs.k8s.io/controller-runtime v0.5.2
)
