package main

import (
	contourv1 "github.com/heptio/contour/apis/contour/v1beta1"
	contourfake "github.com/heptio/contour/apis/generated/clientset/versioned/fake"
	contourinformers "github.com/heptio/contour/apis/generated/informers/externalversions"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	clientgotesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/utils/diff"
	"reflect"
	"testing"
	"time"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
	hostSuffixTest     = "test"
	requestTimeoutTest = "1s"
)

type fixture struct {
	t *testing.T

	contourclient *contourfake.Clientset
	kubeclient    *k8sfake.Clientset
	// Objects to put in the store.
	svcsLister []*corev1.Service
	irsLister  []*contourv1.IngressRoute
	// Actions expected to happen on the client.
	svcsactions []clientgotesting.Action
	irsactions  []clientgotesting.Action
	// Objects from here preloaded into NewSimpleFake.
	svcsobjects []runtime.Object
	irsobjects  []runtime.Object
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{}
	f.t = t
	f.irsobjects = []runtime.Object{}
	f.svcsobjects = []runtime.Object{}
	return f
}

func newSparkDriverService(name string) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: metav1.NamespaceDefault,
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: "v1",
					Name:       "spark-driver-pod-name",
					Kind:       "Pod",
				},
			},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"spark-app-selector": "spark-test",
				"spark-role":         "driver",
			},
			Type: corev1.ServiceTypeClusterIP,
			Ports: []corev1.ServicePort{
				{
					Name:       "driver-rpc-port",
					Port:       7078,
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(7078),
				},
			},
		},
	}
}

func (f *fixture) newController() (*Controller, contourinformers.SharedInformerFactory,
	informers.SharedInformerFactory) {
	f.contourclient = contourfake.NewSimpleClientset(f.irsobjects...)
	f.kubeclient = k8sfake.NewSimpleClientset(f.svcsobjects...)

	contourI := contourinformers.NewSharedInformerFactory(f.contourclient, noResyncPeriodFunc())
	k8sI := informers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())

	c := NewController(hostSuffixTest, requestTimeoutTest, f.kubeclient, f.contourclient, k8sI.Core().V1().Services(),
		contourI.Contour().V1beta1().IngressRoutes())
	c.servicesSynced = alwaysReady
	c.ingressRoutesSynced = alwaysReady

	for _, s := range f.svcsLister {
		k8sI.Core().V1().Services().Informer().GetIndexer().Add(s)
	}

	for _, ir := range f.irsLister {
		contourI.Contour().V1beta1().IngressRoutes().Informer().GetIndexer().Add(ir)
	}
	return c, contourI, k8sI
}

func (f *fixture) run(driverSvcName string) {
	f.runController(driverSvcName, true, false)
}

func (f *fixture) runExpectError(driverSvcName string) {
	f.runController(driverSvcName, true, true)
}

func (f *fixture) runController(serviceName string, startInformers, expectError bool) {
	c, contourI, k8sI := f.newController()
	if startInformers {
		stopCh := make(chan struct{})
		defer close(stopCh)
		contourI.Start(stopCh)
		k8sI.Start(stopCh)
	}

	err := c.syncHandler(serviceName)
	if !expectError && err != nil {
		f.t.Errorf("error syncing service: %v", err)
	} else if expectError && err == nil {
		f.t.Error("expected error syncing ")
	}

	irsactions := filterInformerActions(f.contourclient.Actions())
	for i, action := range irsactions {
		if len(f.irsactions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(irsactions)-len(f.irsactions),
				irsactions[i:])
			break
		}
		expectedAction := f.irsactions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.irsactions) > len(irsactions) {
		f.t.Errorf("%d additional expected actions: %+v",
			len(f.irsactions)-len(irsactions), f.irsactions[len(irsactions):])
	}

	svcsctions := filterInformerActions(f.kubeclient.Actions())
	for i, action := range svcsctions {
		if len(f.svcsactions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(svcsctions)-len(f.svcsactions), svcsctions[i:])
			break
		}

		expectedAction := f.svcsactions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.svcsactions) > len(svcsctions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.svcsactions)-len(svcsctions), f.svcsactions[len(svcsctions):])
	}
}

// filterInformerActions filters list and watch actions for testing resources.
// Since list and watch don't change resource state we can filter it to lower
// nose level in our tests.
func filterInformerActions(actions []clientgotesting.Action) []clientgotesting.Action {
	ret := []clientgotesting.Action{}
	for _, action := range actions {
		if len(action.GetNamespace()) == 0 &&
			(action.Matches("list", "services") ||
				action.Matches("watch", "services") ||
				action.Matches("list", "ingressroutes") ||
				action.Matches("watch", "ingressroutes")) {
			continue
		}
		ret = append(ret, action)
	}

	return ret
}

// checkAction verifies that expected and actual actions are equal and both have
// same attached resources
func checkAction(expected, actual clientgotesting.Action, t *testing.T) {
	if !(expected.Matches(actual.GetVerb(), actual.GetResource().Resource) &&
		actual.GetSubresource() == expected.GetSubresource()) {
		t.Errorf("Expected\n\t%#v\ngot\n\t%#v", expected, actual)
		return
	}

	if reflect.TypeOf(actual) != reflect.TypeOf(expected) {
		t.Errorf("Action has wrong type. Expected: %t. Got: %t", expected, actual)
		return
	}

	switch a := actual.(type) {
	case clientgotesting.CreateAction:
		e, _ := expected.(clientgotesting.CreateAction)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintDiff(expObject, object))
		}
	case clientgotesting.UpdateAction:
		e, _ := expected.(clientgotesting.UpdateAction)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintDiff(expObject, object))
		}
	case clientgotesting.PatchAction:
		e, _ := expected.(clientgotesting.PatchAction)
		expPatch := e.GetPatch()
		patch := a.GetPatch()

		if !reflect.DeepEqual(expPatch, patch) {
			t.Errorf("Action %s %s has wrong patch\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintDiff(expPatch, patch))
		}
	}
}

func (f *fixture) expectCreateSparkUIServiceAction(svc *corev1.Service) {
	f.svcsactions = append(f.svcsactions, clientgotesting.NewCreateAction(schema.
	GroupVersionResource{Resource: "services"}, svc.Namespace, svc))
}

func (f *fixture) expectUpdateSparkDriverServceAction(svc *corev1.Service) {
	f.svcsactions = append(f.svcsactions, clientgotesting.NewUpdateAction(schema.
	GroupVersionResource{Resource: "services"}, svc.Namespace, svc))
}

func (f *fixture) expectCreateSparkUIIngressRouteAction(ir *contourv1.IngressRoute) {
	f.irsactions = append(f.irsactions, clientgotesting.NewCreateAction(schema.
	GroupVersionResource{Resource: "ingressroutes"}, ir.Namespace, ir))
}

func getKey(driverService *corev1.Service, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(driverService)
	if err != nil {
		t.Errorf("Unexpected error getting key for service %v: %v",
			driverService.Name, err)
		return ""
	}
	return key
}

func TestCreatesSparkDriverService(t *testing.T) {
	f := newFixture(t)
	driverService := newSparkDriverService("test-driver-svc")

	f.svcsLister = append(f.svcsLister, driverService)
	f.svcsobjects = append(f.svcsobjects, driverService)

	expSparkUISvc := NewSparkUIService(driverService)
	expIngressRoute := NewSparkUIIngressRoute(expSparkUISvc, hostSuffixTest, driverService)
	f.expectCreateSparkUIServiceAction(expSparkUISvc)
	f.expectCreateSparkUIIngressRouteAction(expIngressRoute)

	f.run(getKey(driverService, t))
}
