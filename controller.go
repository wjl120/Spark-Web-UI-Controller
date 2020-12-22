package main

import (
	"fmt"
	contourv1 "github.com/heptio/contour/apis/contour/v1beta1"
	contourclientset "github.com/heptio/contour/apis/generated/clientset/versioned"
	contourinformerssv1 "github.com/heptio/contour/apis/generated/informers/externalversions/contour/v1beta1"
	contourlistersv1 "github.com/heptio/contour/apis/generated/listers/contour/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformerv1 "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	corelisterv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog"
	"strings"
	"time"
)

const (
	driverServiceSuffix  = "-driver-svc"
	sparkUIServiceSuffix = "-ui-svc"
	ingressRouteSuffix   = "-ingress"
)

type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset       kubernetes.Interface
	contourclientset    contourclientset.Interface
	servicesSynced      cache.InformerSynced
	servicesLister      corelisterv1.ServiceLister
	ingressRoutesSynced cache.InformerSynced
	ingressRoutesLister contourlistersv1.IngressRouteLister
	workqueue           workqueue.RateLimitingInterface
	hostsuffix          string
	requestTimeout      string
}

// Run is the main path of execution for the controller loop
func (c *Controller) Run(threadiness int, stopCh <-chan struct{}) error {
	defer runtime.HandleCrash()
	defer c.workqueue.ShutDown()

	// do the initial synchronization (one time) to populate resources
	if ok := cache.WaitForCacheSync(stopCh, c.HasSynced); !ok {
		return fmt.Errorf("Error syncing cache")

	}
	klog.Info("Starting workers")
	for i := 0; i < threadiness; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}
	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")
	return nil
}

// NewController returns a new sample controller
func NewController(
	hostsuffix string,
	requestTimeout string,
	kubeclientset kubernetes.Interface,
	contourclientset contourclientset.Interface,
	servicesInformer coreinformerv1.ServiceInformer,
	ingressRoutesInformer contourinformerssv1.IngressRouteInformer) *Controller {

	queue := workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())

	servicesInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			klog.Infof("Add service: %s", key)
			if err == nil {
				queue.Add(key)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			klog.Infof("Update service: %s", key)
			if err == nil {
				queue.Add(key)
			}
		},
		DeleteFunc: func(obj interface{}) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			klog.Infof("Delete service: %s", key)
			if err == nil {
				queue.Add(key)
			}
		},
	})
	controller := &Controller{
		kubeclientset:       kubeclientset,
		contourclientset:    contourclientset,
		servicesSynced:      servicesInformer.Informer().HasSynced,
		servicesLister:      servicesInformer.Lister(),
		ingressRoutesSynced: ingressRoutesInformer.Informer().HasSynced,
		ingressRoutesLister: ingressRoutesInformer.Lister(),
		workqueue:           queue,
		hostsuffix:          hostsuffix,
		requestTimeout:      requestTimeout,
	}
	return controller
}
func (c *Controller) HasSynced() bool {
	return c.servicesSynced() && c.ingressRoutesSynced()
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue. 
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}

}
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()
	if shutdown {
		return false
	}
	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			runtime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the Service
		// resource to be synced.
		if err := c.syncHandler(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		runtime.HandleError(err)
		return true
	}
	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the ? resource
// with the current status of the resource.
func (c *Controller) syncHandler(key string) error {
	namespace, name, err := cache.SplitMetaNamespaceKey(key)

	if err != nil {
		runtime.HandleError(fmt.Errorf("invalid resource key: %s", key))
		return nil
	}

	// spark driver svc should end with driverServiceSuffix
	if !strings.HasSuffix(name, driverServiceSuffix) {
		klog.Infof("Get service: %s, not end with %s, ignoring it", name, driverServiceSuffix)
		return nil
	}

	service, err := c.servicesLister.Services(namespace).Get(name)
	if err != nil {
		if errors.IsNotFound(err) {
			runtime.HandleError(fmt.Errorf("service '%s' in work queue no longer exists", key))
			// this should a spark driver service deleted event.
			// service not found depend owner reference to garbage collect ui service and ingress route
			return nil
		}
		return err
	}
	// spark driver svc should has a selector spark-role: driver
	if service.Spec.Selector["spark-role"] != "driver" {
		klog.Infof("Get service: %s, has not a selector spark-role: driver, ignoring it", service)
		return nil
	}

	//check
	err2 := c.createSparkUIServiceIfNotExists(namespace, name)

	return err2
}

// spark ui name without namespace
func getSparkUIServiceName(name string) string {
	return strings.Replace(name, driverServiceSuffix, sparkUIServiceSuffix, 1)
}

// spark ui ingressroute name without namespace from spark ui svc name
func (c *Controller) getSparkUIIngressRouteName(name string) string {
	return name + ingressRouteSuffix
}

//create spark ui and ingress route from driver svc namespace and name
func (c *Controller) createSparkUIServiceIfNotExists(namespace, name string) error {
	sparkUIServiceName := getSparkUIServiceName(name)
	_, err1 := c.servicesLister.Services(namespace).Get(sparkUIServiceName)
	if err1 != nil {
		if errors.IsNotFound(err1) {
			klog.Infof("spark ui service with name: %s is not found, now create one ...", sparkUIServiceName)
			driver, err2 := c.servicesLister.Services(namespace).Get(name)
			if err2 == nil {
				uiService, err3 := c.kubeclientset.CoreV1().Services(namespace).Create(NewSparkUIService(driver))
				if err3 != nil {
					return err3
				}
				// if spark ui service is not defined, related ingress route should not exists too.
				ingressName := c.getSparkUIIngressRouteName(sparkUIServiceName)
				ingressRoute, err4 := c.ingressRoutesLister.IngressRoutes(namespace).Get(ingressName)
				if err4 != nil {
					if errors.IsNotFound(err4) {
						klog.Infof("spark ui ingress route with name: %s is not found, now create one ...", ingressName)
						_, _ = c.contourclientset.ContourV1beta1().IngressRoutes(namespace).Create(
							NewSparkUIIngressRoute(uiService, c.hostsuffix, driver))
					}
				} else {
					klog.Infof("spark ui ingress route with name: %s already exists", ingressRoute.Name)
					return nil
				}

			}
			return err2
		}
		return err1
	}
	return nil
}

// construct spark ui Service from driver service namespace and name
func NewSparkUIService(driver *corev1.Service) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            getSparkUIServiceName(driver.Name),
			Namespace:       driver.Namespace,
			OwnerReferences: driver.OwnerReferences,
		},
		Spec: corev1.ServiceSpec{
			Selector: driver.Spec.Selector,
			Type:     driver.Spec.Type,
			Ports: []corev1.ServicePort{
				{
					Name:       "spark-driver-ui-port",
					Port:       4040,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(4040),
				},
			},
		},
	}

}
func NewSparkUIIngressRoute(uiService *corev1.Service, hostsuffix string,
	driver *corev1.Service) *contourv1.IngressRoute {
	return &contourv1.IngressRoute{
		ObjectMeta: metav1.ObjectMeta{
			Name:            uiService.Name + ingressRouteSuffix,
			Namespace:       uiService.Namespace,
			OwnerReferences: uiService.OwnerReferences,
		},
		Spec: contourv1.IngressRouteSpec{
			Routes: []contourv1.Route{
				{
					Match: "/",
					TimeoutPolicy: &contourv1.TimeoutPolicy{
						Request: requestTimeout,
					},
					Services: []contourv1.Service{
						{
							Name: uiService.Name,
							Port: int(uiService.Spec.Ports[0].Port),
						},
					},
				},
			},
			VirtualHost: &contourv1.VirtualHost{
				Fqdn: driver.Name + hostsuffix,
			},
		},
	}

}
