// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

// NOTE: NOT READY AT ALL - FIX BUT DON'T USE

package controllers

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	apps "k8s.io/api/apps/v1"
	coreapi "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/diff"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"

	ndbcontroller "github.com/ocklin/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/ocklin/ndb-operator/pkg/generated/clientset/versioned/fake"
	informers "github.com/ocklin/ndb-operator/pkg/generated/informers/externalversions"
)

var (
	alwaysReady        = func() bool { return true }
	noResyncPeriodFunc = func() time.Duration { return 0 }
)

type fixture struct {
	t *testing.T

	client     *fake.Clientset
	kubeclient *k8sfake.Clientset

	// stop channel
	stopCh chan struct{}

	sif  informers.SharedInformerFactory
	k8If kubeinformers.SharedInformerFactory

	// Objects to put in the store.
	ndbLister        []*ndbcontroller.Ndb
	deploymentLister []*apps.Deployment
	configMapLister  []*coreapi.ConfigMap

	// Actions expected to happen on the client.
	kubeactions []core.Action
	actions     []core.Action
	// Objects from here preloaded into NewSimpleFake.
	kubeobjects []runtime.Object
	objects     []runtime.Object
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{}
	f.t = t
	f.objects = []runtime.Object{}
	f.kubeobjects = []runtime.Object{}

	return f
}

func (f *fixture) init() {

	f.client = fake.NewSimpleClientset(f.objects...)
	f.kubeclient = k8sfake.NewSimpleClientset(f.kubeobjects...)

	f.sif = informers.NewSharedInformerFactory(f.client, noResyncPeriodFunc())
	f.k8If = kubeinformers.NewSharedInformerFactory(f.kubeclient, noResyncPeriodFunc())

	f.stopCh = make(chan struct{})
}

func (f *fixture) start() {
	// start informers
	f.sif.Start(f.stopCh)
	f.k8If.Start(f.stopCh)
}

func (f *fixture) close() {
	klog.Info("Closing fixture")
	close(f.stopCh)
}

func newNdb(namespace string, name string, noofnodes int) *ndbcontroller.Ndb {
	return &ndbcontroller.Ndb{
		TypeMeta: metav1.TypeMeta{APIVersion: ndbcontroller.SchemeGroupVersion.String()},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: ndbcontroller.NdbSpec{
			DeploymentName: fmt.Sprintf("%s-deployment", name),
			Ndbd: ndbcontroller.NdbNdbdSpec{
				NodeCount:    int32Ptr(int32(noofnodes)),
				NoOfReplicas: int32Ptr(int32(2)),
			},
			Mgmd: ndbcontroller.NdbMgmdSpec{
				NodeCount: int32Ptr(int32(noofnodes)),
			},
		},
	}
}

func (f *fixture) newController() *Controller {

	c := NewController(f.kubeclient, f.client,
		f.k8If.Apps().V1().StatefulSets(),
		f.k8If.Core().V1().Services(),
		f.k8If.Core().V1().Pods(),
		f.k8If.Core().V1().ConfigMaps(),
		f.sif.Ndbcontroller().V1alpha1().Ndbs())

	for _, n := range f.ndbLister {
		f.sif.Ndbcontroller().V1alpha1().Ndbs().Informer().GetIndexer().Add(n)
	}

	for _, d := range f.configMapLister {
		f.k8If.Core().V1().ConfigMaps().Informer().GetIndexer().Add(d)
	}

	return c
}

func (f *fixture) run(fooName string) {
	f.runController(fooName, true, false)
}

func (f *fixture) runExpectError(fooName string) {
	f.runController(fooName, true, true)
}

func (f *fixture) runController(fooName string, startInformers bool, expectError bool) {
	c := f.newController()
	if startInformers {
		f.start()
	}

	err := c.syncHandler(fooName)
	if !expectError && err != nil {
		f.t.Errorf("error syncing ndb: %v", err)
	} else if expectError && err == nil {
		f.t.Error("expected error syncing ndb, got nil")
	}
	klog.Infof("Successfully syncing ndb")

	filterInformerActions(f.client.Actions())
	actions := filterInformerActions(f.client.Actions())
	for i, action := range actions {
		if len(f.actions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(actions)-len(f.actions), actions[i:])
			break
		}

		expectedAction := f.actions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.actions) > len(actions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.actions)-len(actions), f.actions[len(actions):])
	}

	k8sActions := filterInformerActions(f.kubeclient.Actions())
	for i, action := range k8sActions {
		if len(f.kubeactions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(k8sActions)-len(f.kubeactions), k8sActions[i:])
			break
		}

		expectedAction := f.kubeactions[i]
		checkAction(expectedAction, action, f.t)
	}

	if len(f.kubeactions) > len(k8sActions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.kubeactions)-len(k8sActions), f.kubeactions[len(k8sActions):])
	}
}

// checkAction verifies that expected and actual actions are equal and both have
// same attached resources
func checkAction(expected, actual core.Action, t *testing.T) {
	if !(expected.Matches(actual.GetVerb(), actual.GetResource().Resource) && actual.GetSubresource() == expected.GetSubresource()) {
		t.Errorf("Expected\n\t%#v\ngot\n\t%#v", expected, actual)
		return
	}

	if reflect.TypeOf(actual) != reflect.TypeOf(expected) {
		t.Errorf("Action has wrong type. Expected: %t. Got: %t", expected, actual)
		return
	}

	switch a := actual.(type) {
	case core.CreateActionImpl:
		e, _ := expected.(core.CreateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.UpdateActionImpl:
		e, _ := expected.(core.UpdateActionImpl)
		expObject := e.GetObject()
		object := a.GetObject()

		if !reflect.DeepEqual(expObject, object) {
			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
		}
	case core.PatchActionImpl:
		e, _ := expected.(core.PatchActionImpl)
		expPatch := e.GetPatch()
		patch := a.GetPatch()

		if !reflect.DeepEqual(expPatch, patch) {
			t.Errorf("Action %s %s has wrong patch\nDiff:\n %s",
				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expPatch, patch))
		}
	default:
		t.Errorf("Uncaptured Action %s %s, you should explicitly add a case to capture it",
			actual.GetVerb(), actual.GetResource().Resource)
	}
}

// filterInformerActions filters list and watch actions for testing resources.
// Since list and watch don't change resource state we can filter it to lower
// nose level in our tests.
func filterInformerActions(actions []core.Action) []core.Action {
	//klog.Infof("Filtering %d actions", len(actions))
	ret := []core.Action{}
	for _, action := range actions {
		if len(action.GetNamespace()) == 0 &&
			(action.Matches("list", "ndbs") ||
				action.Matches("watch", "ndbs") ||
				action.Matches("list", "pods") ||
				action.Matches("watch", "pods") ||
				action.Matches("list", "services") ||
				action.Matches("watch", "services") ||
				action.Matches("list", "configmaps") ||
				action.Matches("watch", "configmaps") ||
				action.Matches("list", "statefulsets") ||
				action.Matches("watch", "statefulsets")) {
			//klog.Infof("Filtering +%v", action)
			continue
		}
		//klog.Infof("Appending +%v", action)
		ret = append(ret, action)
	}

	return ret
}

func (f *fixture) expectCreateAction(ns string, resource string, o runtime.Object) {
	f.kubeactions = append(f.kubeactions, core.NewCreateAction(schema.GroupVersionResource{Resource: resource}, ns, o))
}

func (f *fixture) expectUpdateAction(ns string, resource string, o runtime.Object) {
	f.kubeactions = append(f.kubeactions, core.NewUpdateAction(schema.GroupVersionResource{Resource: resource}, ns, o))
}

func (f *fixture) expectUpdateNdbStatusAction(ndb *ndbcontroller.Ndb) {
	action := core.NewUpdateAction(schema.GroupVersionResource{Resource: "ndbs"}, ndb.Namespace, ndb)
	// TODO: before #38113 was merged, we can't use Subresource
	action.Subresource = "status"
	f.actions = append(f.actions, action)
}

func getKey(foo *ndbcontroller.Ndb, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(foo)
	if err != nil {
		t.Errorf("Unexpected error getting key for foo %v: %v", foo.Name, err)
		return ""
	}
	return key
}

func TestCreatesCluster(t *testing.T) {

	f := newFixture(t)
	defer f.close()

	ns := metav1.NamespaceDefault
	ndb := newNdb(ns, "test", 1)

	// we first need to set up arrays with objects ...
	f.ndbLister = append(f.ndbLister, ndb)
	f.objects = append(f.objects, ndb)

	// ... before we init the fake clients with those objects.
	// objects not listed in arrays at fakeclient setup will eventually be deleted
	f.init()

	// update labels will happen first sync run
	f.expectUpdateAction(ns, "ndbs", ndb)

	// two services for ndbd and mgmds
	f.expectCreateAction(ns, "services", ndb)
	f.expectCreateAction(ns, "services", ndb)
	f.expectUpdateNdbStatusAction(ndb)

	f.run(getKey(ndb, t))
}

/*

func TestDoNothing(t *testing.T) {
	f := newFixture(t)
	foo := newNdb("test", 1)

		d := newDeployment(foo)

		f.ndbLister = append(f.ndbLister, foo)
		f.objects = append(f.objects, foo)
		f.deploymentLister = append(f.deploymentLister, d)
		f.kubeobjects = append(f.kubeobjects, d)

		f.expectUpdateFooStatusAction(foo)
		f.run(getKey(foo, t))
}

func TestUpdateDeployment(t *testing.T) {
	f := newFixture(t)
	foo := newNdb("test", 1)

		d := newDeployment(foo)

		// Update replicas
		foo.Spec.NodeCount = int32Ptr(2)
		expDeployment := newDeployment(foo)

		f.ndbLister = append(f.ndbLister, foo)
		f.objects = append(f.objects, foo)
		f.deploymentLister = append(f.deploymentLister, d)
		f.kubeobjects = append(f.kubeobjects, d)

		f.expectUpdateFooStatusAction(foo)
		f.expectUpdateDeploymentAction(expDeployment)
		f.run(getKey(foo, t))
}

func TestNotControlledByUs(t *testing.T) {
	f := newFixture(t)
	foo := newNdb("test", 1)

		d := newDeployment(foo)

		d.ObjectMeta.OwnerReferences = []metav1.OwnerReference{}

		f.ndbLister = append(f.ndbLister, foo)
		f.objects = append(f.objects, foo)
		f.deploymentLister = append(f.deploymentLister, d)
		f.kubeobjects = append(f.kubeobjects, d)

		f.runExpectError(getKey(foo, t))
}

*/

func int32Ptr(i int32) *int32 { return &i }
