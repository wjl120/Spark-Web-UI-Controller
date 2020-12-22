package main

import (
	"flag"
	contourclientset "github.com/heptio/contour/apis/generated/clientset/versioned"
	contourinformers "github.com/heptio/contour/apis/generated/informers/externalversions"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog"
	"time"
)

var (
	masterURL      string
	kubeconfig     string
	hostSuffix     string
	requestTimeout string
)

func main() {
	klog.InitFlags(nil)
	flag.Parse()

	// set up signals so we handle the first shutdown signal gracefully
	stopCh := make(chan struct{})
	defer close(stopCh)

	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %s", err.Error())
	}

	kubeClient, err := kubernetes.NewForConfig(cfg)
	contourClient, err := contourclientset.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building kubernetes clientset: %s", err.Error())
	}

	informerFactory := informers.NewSharedInformerFactory(kubeClient, time.Second*30)
	serviceInformer := informerFactory.Core().V1().Services()

	contourInformerFactory := contourinformers.NewSharedInformerFactory(contourClient, time.Second*30)
	ingressRouteInformer := contourInformerFactory.Contour().V1beta1().IngressRoutes()

	controller := NewController(hostSuffix, requestTimeout, kubeClient, contourClient, serviceInformer,
		ingressRouteInformer)

	//notice that there is no need to run Start mothods in a separate goroutine. (i.e. go kubeInformerFactory.Start(
	// stopCh)
	//Start method is non-blocking and runs all registered informers in a dedicated goroutine.
	informerFactory.Start(stopCh)
	contourInformerFactory.Start(stopCh)

	if err = controller.Run(2, stopCh); err != nil {
		klog.Fatalf("Error running controller: %s", err.Error())
	}

}
// 
func init() {
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&hostSuffix, "hostsuffix", ".spark-ui.ushareit.me", "the host suffix ,"+
		"example .spark-ui.ushareit.org ")
	flag.StringVar(&requestTimeout, "request_timeout", "60s", "envoy request spark ui timeout.")
}
