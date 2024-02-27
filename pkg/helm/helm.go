package helm

import (
	"os"

	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/cli"
)

var settings *cli.EnvSettings

type HelmAct interface {
	Apply(rest *rest.Config) error
	Uninstall(rest *rest.Config) error
	Upgrade(rest *rest.Config) error
	List(rest *rest.Config) (status string, exists bool)
}

type Helm struct {
	Action      *action.Configuration
	ReleaseName string
	Namespace   string
	Values      []string
	RepoName    string
	ChartName   string
	RepoUrl     string
}

// HelmList Method installs the chart.
// https://helm.sh/docs/topics/advanced/#simple-example
func (h *Helm) List(rest *rest.Config) (status string, exists bool) {

	settings := cli.New()
	restGetter := NewRESTClientGetter(rest, h.Namespace)

	if err := h.Action.Init(&restGetter, h.Namespace, os.Getenv("HELM_DRIVER"), klog.Infof); err != nil {
		return "", false
	}

	clientList := action.NewList(h.Action)
	settings.EnvVars()

	// Only list deployed
	clientList.Deployed = true
	results, err := clientList.Run()
	if err != nil {
		return "", false
	}

	for _, result := range results {
		if result.Name == h.ReleaseName {
			return result.Info.Status.String(), true

		}
	}
	return "", false
}

type simpleRESTClientGetter struct {
	config    *rest.Config
	namespace string
}

func NewRESTClientGetter(config *rest.Config, namespace string) simpleRESTClientGetter {
	return simpleRESTClientGetter{
		namespace: namespace,
		config:    config,
	}
}

func (c *simpleRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	return c.config, nil
}

func (c *simpleRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config, err := c.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	config.Burst = 100

	discoveryClient, _ := discovery.NewDiscoveryClientForConfig(config)
	return memory.NewMemCacheClient(discoveryClient), nil
}

func (c *simpleRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := c.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	expander := restmapper.NewShortcutExpander(mapper, discoveryClient)
	return expander, nil
}

func (c *simpleRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig

	overrides := &clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}
	overrides.Context.Namespace = c.namespace

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
}
