// Copyright (c) 2020, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

// NOTE: NOT READY AT ALL - FIX BUT DON'T USE

package controllers

import (
	"context"
	"reflect"
	"testing"
	"time"

	apps "k8s.io/api/apps/v1"
	appsv1 "k8s.io/api/apps/v1"
	coreapi "k8s.io/api/core/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog"
	"k8s.io/utils/diff"

	ndbcontroller "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1alpha1"
	"github.com/mysql/ndb-operator/pkg/generated/clientset/versioned/fake"
	informers "github.com/mysql/ndb-operator/pkg/generated/informers/externalversions"
	helpers "github.com/mysql/ndb-operator/pkg/helpers"
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

	c *Controller
}

func newFixtures(t *testing.T, ndbs []*ndbcontroller.Ndb) *fixture {

	f := &fixture{}
	f.t = t
	f.objects = []runtime.Object{}
	f.kubeobjects = []runtime.Object{}

	// we first need to set up arrays with objects ...
	if len(f.ndbLister) > 0 {
		t.Errorf("f.ndbLister len was %d", len(f.ndbLister))
	}
	if len(f.objects) > 0 {
		t.Errorf("f.objects len was %d", len(f.objects))
	}

	for _, ndb := range ndbs {
		f.ndbLister = append(f.ndbLister, ndb)
		f.objects = append(f.objects, ndb)
	}

	// ... before we init the fake clients with those objects.
	// objects not listed in arrays at fakeclient setup will eventually be deleted
	f.init()

	return f
}

func newFixture(t *testing.T, ndb *ndbcontroller.Ndb) *fixture {
	ndbs := make([]*ndbcontroller.Ndb, 1)
	ndbs[0] = ndb
	return newFixtures(t, ndbs)
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

func (f *fixture) newController() {

	cc := NewControllerContext(f.kubeclient, f.client, false)
	f.c = NewController(cc,
		f.k8If.Apps().V1().StatefulSets(),
		f.k8If.Apps().V1().Deployments(),
		f.k8If.Core().V1().Services(),
		f.k8If.Core().V1().Pods(),
		f.k8If.Core().V1().ConfigMaps(),
		f.sif.Mysql().V1alpha1().Ndbs())

	for _, n := range f.ndbLister {
		f.sif.Mysql().V1alpha1().Ndbs().Informer().GetIndexer().Add(n)
	}

	for _, d := range f.configMapLister {
		f.k8If.Core().V1().ConfigMaps().Informer().GetIndexer().Add(d)
	}
}

func (f *fixture) run(fooName string) {
	f.setupController(fooName, true)
	f.runController(fooName, false)
}

func (f *fixture) runExpectError(fooName string) {
	f.setupController(fooName, true)
	f.runController(fooName, true)
}

func (f *fixture) setupController(fooName string, startInformers bool) {
	f.newController()
	if startInformers {
		f.start()
	}
}

func (f *fixture) runController(fooName string, expectError bool) {

	err := f.c.syncHandler(fooName)

	if !expectError && err != nil {
		f.t.Errorf("error syncing ndb: %v", err)
	} else if expectError {
		if err == nil {
			f.t.Error("expected error syncing ndb, got nil")
		} else {
			klog.Infof("Expected error and received one (good): %s", err)
		}
	} else {
		klog.Infof("Successfully syncing ndb")
	}

	actions := filterInformerActions(f.client.Actions())
	k8sActions := filterInformerActions(f.kubeclient.Actions())

	for i, action := range actions {

		if len(f.actions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(actions)-len(f.actions), actions[i:])
			break
		}

		expectedAction := f.actions[i]

		/*
			s, _ := json.Marshal(expectedAction)
			fmt.Printf("[%d] %d : %s\n", i, len(f.actions), s)
			s, _ = json.Marshal(action)
			fmt.Printf("[%d] %d : %s\n\n", i, len(f.actions), s)
		*/

		checkAction(expectedAction, action, f.t)
	}

	if len(f.actions) > len(actions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.actions)-len(actions), f.actions[len(actions):])
	}

	for i, action := range k8sActions {

		if len(f.kubeactions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(k8sActions)-len(f.kubeactions), k8sActions[i:])
			break
		}

		expectedAction := f.kubeactions[i]
		checkAction(expectedAction, action, f.t)
		/*
			s, _ := json.Marshal(expectedAction)
			fmt.Printf("[%d] %d : %s\n", i, len(f.kubeactions), s)
			s, _ = json.Marshal(action)
			fmt.Printf("[%d] %d : %s\n\n", i, len(f.kubeactions), s)
		*/
	}

	if len(f.kubeactions) > len(k8sActions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.kubeactions)-len(k8sActions), f.kubeactions[len(k8sActions):])
	}
}

func extractObjectMetaData(actual core.Action, extO, actO runtime.Object, t *testing.T) (metav1.ObjectMeta, metav1.ObjectMeta) {

	var expOM, actOM metav1.ObjectMeta
	switch actual.GetResource().Resource {
	case "configmaps":
		expOM = extO.(*corev1.ConfigMap).ObjectMeta
		actOM = actO.(*corev1.ConfigMap).ObjectMeta
	case "statefulsets":
		expOM = extO.(*appsv1.StatefulSet).ObjectMeta
		actOM = actO.(*appsv1.StatefulSet).ObjectMeta
	case "poddisruptionbudgets":
		expOM = extO.(*policyv1beta1.PodDisruptionBudget).ObjectMeta
		actOM = actO.(*policyv1beta1.PodDisruptionBudget).ObjectMeta
	case "services":
		expOM = extO.(*corev1.Service).ObjectMeta
		actOM = actO.(*corev1.Service).ObjectMeta
	case "deployments":
		expOM = extO.(*appsv1.Deployment).ObjectMeta
		actOM = actO.(*appsv1.Deployment).ObjectMeta
	case "ndbs":
		expOM = extO.(*ndbcontroller.Ndb).ObjectMeta
		actOM = actO.(*ndbcontroller.Ndb).ObjectMeta
	default:
		t.Errorf("Action has unkown type. Got: %s", actual.GetResource().Resource)
	}

	return expOM, actOM
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

		expOM, actOM := extractObjectMetaData(actual, e.GetObject(), a.GetObject(), t)

		if expOM.Name != actOM.Name {
			t.Errorf("Action %s %s has wrong name %s\n",
				a.GetVerb(), a.GetResource().Resource, actOM.Name)
		}

		// lets only compare if expected labels are all found in actual labels

		for expK, expV := range expOM.Labels {
			if actV, ok := actOM.Labels[expK]; ok {
				if expV != actV {
					t.Errorf("Action %s %s has wrong label value for key %s: %s, expected: %s\n",
						a.GetVerb(), a.GetResource().Resource, expK, actV, expV)
				}
			} else {
				t.Errorf("Action %s %s misses must have label key %s\n",
					a.GetVerb(), a.GetResource().Resource, expK)
			}
		}

		if !reflect.DeepEqual(expOM.OwnerReferences, actOM.OwnerReferences) {
			t.Errorf("Action %s %s has wrong owner reference %s\n",
				a.GetVerb(), a.GetResource().Resource,
				diff.ObjectGoPrintSideBySide(expOM.OwnerReferences, actOM.OwnerReferences))
		}

		/*
			if !reflect.DeepEqual(expObject, object) {
				t.Errorf("Action %s %s has wrong object\n",
					a.GetVerb(), a.GetResource().Resource)
				//			t.Errorf("Action %s %s has wrong object\nDiff:\n %s",
				//				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expObject, object))
			}
		*/
	case core.UpdateActionImpl:
		//e, _ := expected.(core.UpdateActionImpl)

	case core.PatchActionImpl:
		e, _ := expected.(core.PatchActionImpl)
		expPatch := e.GetPatch()
		patch := a.GetPatch()

		if !reflect.DeepEqual(expPatch, patch) {
			t.Errorf("Action %s %s has wrong patch\n",
				a.GetVerb(), a.GetResource().Resource)
			//				t.Errorf("Action %s %s has wrong patch\nDiff:\n %s",
			//				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expPatch, patch))
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
		if action.GetNamespace() == "default" &&
			(action.Matches("get", "ndbs") ||
				action.Matches("get", "pods") ||
				action.Matches("list", "pods") ||
				action.Matches("get", "services") ||
				action.Matches("get", "configmaps") ||
				action.Matches("get", "poddisruptionbudgets") ||
				action.Matches("get", "deployments") ||
				action.Matches("get", "statefulsets")) {
			//klog.Infof("Filtering +%v", action)
			continue
		}
		if len(action.GetNamespace()) == 0 &&
			(action.Matches("list", "ndbs") ||
				action.Matches("watch", "ndbs") ||
				action.Matches("list", "pods") ||
				action.Matches("watch", "pods") ||
				action.Matches("list", "services") ||
				action.Matches("watch", "services") ||
				action.Matches("list", "configmaps") ||
				action.Matches("watch", "configmaps") ||
				action.Matches("list", "poddisruptionbudgets") ||
				action.Matches("watch", "poddisruptionbudgets") ||
				action.Matches("list", "deployments") ||
				action.Matches("watch", "deployments") ||
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

func (f *fixture) expectCreateAction(ns string, group, version, resource string, o runtime.Object) {
	grpVersionResource := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	f.kubeactions = append(f.kubeactions, core.NewCreateAction(grpVersionResource, ns, o))
}

func (f *fixture) expectUpdateAction(ns string, resource string, o runtime.Object) {
	f.kubeactions = append(f.kubeactions, core.NewUpdateAction(schema.GroupVersionResource{Resource: resource}, ns, o))
}

func (f *fixture) expectUpdateNdbAction(ns string, o runtime.Object) {
	grpVersionResource := schema.GroupVersionResource{Group: "mysql.oracle.com", Version: "v1alpha1", Resource: "ndbs"}
	action := core.NewUpdateAction(grpVersionResource, ns, o)
	f.actions = append(f.actions, action)
}

func (f *fixture) expectUpdateNdbStatusAction(ns string, ndb *ndbcontroller.Ndb) {
	grpVersionResource := schema.GroupVersionResource{Group: "mysql.oracle.com", Version: "v1alpha1", Resource: "ndbs"}
	action := core.NewUpdateAction(grpVersionResource, ns, ndb)
	// TODO: before #38113 was merged, we can't use Subresource
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

func getObjectMetadata(name string, ndb *ndbcontroller.Ndb, t *testing.T) *metav1.ObjectMeta {

	gvk := schema.GroupVersionKind{
		Group:   ndbcontroller.SchemeGroupVersion.Group,
		Version: ndbcontroller.SchemeGroupVersion.Version,
		Kind:    "Ndb",
	}

	return &metav1.ObjectMeta{
		Labels: ndb.GetLabels(),
		Name:   name,
		OwnerReferences: []metav1.OwnerReference{
			*metav1.NewControllerRef(ndb, gvk),
		},
	}
}

func TestCreateInvalidCluster(t *testing.T) {

	ns := metav1.NamespaceDefault
	ndb := helpers.NewTestNdb(ns, "test", 2)

	// that would be an invalid config with 3 replica and 2 data nodes
	ndb.Spec.RedundancyLevel = helpers.IntToInt32Ptr(int(3))

	f := newFixture(t, ndb)
	defer f.close()

	// even if config is invalid we do still expect services / labels to be created / updated
	// but no further resources

	// update labels will happen first sync run
	f.expectUpdateNdbAction(ns, ndb)

	// two services for ndbd and mgmds
	omd := getObjectMetadata("test-mgmd", ndb, t)
	f.expectCreateAction(ns, "", "v1", "services", &corev1.Service{ObjectMeta: *omd})

	omd.Name = "test-mgmd-ext"
	f.expectCreateAction(ns, "", "v1", "services", &corev1.Service{ObjectMeta: *omd})

	omd.Name = "test-ndbd"
	f.expectCreateAction(ns, "", "v1", "services", &corev1.Service{ObjectMeta: *omd})

	omd.Name = "test-mysqld-ext"
	f.expectCreateAction(ns, "", "v1", "services", &corev1.Service{ObjectMeta: *omd})

	f.run(getKey(ndb, t))

	klog.Infof("Fixed invalid config and run again")

	// internally fake client maintains a copy, so we need to update the Ndb object
	ndb.Spec.RedundancyLevel = helpers.IntToInt32Ptr(int(2))
	_, err := f.client.MysqlV1alpha1().Ndbs(ndb.Namespace).Update(context.TODO(), ndb, metav1.UpdateOptions{})

	if err != nil {
		t.Error("Failed to update Ndb resource object")
	}

	// update from above needs to be expected
	f.expectUpdateNdbAction(ns, ndb)

	// run again, this time without error
	f.runController(getKey(ndb, t), false)
}

func TestCreatesCluster(t *testing.T) {

	ns := metav1.NamespaceDefault
	ndb := helpers.NewTestNdb(ns, "test", 2)

	f := newFixture(t, ndb)
	defer f.close()

	// update labels will happen first sync run
	f.expectUpdateNdbAction(ns, ndb)

	// two services for ndbd and mgmds
	omd := getObjectMetadata("test-mgmd", ndb, t)
	f.expectCreateAction(ns, "", "v1", "services", &corev1.Service{ObjectMeta: *omd})

	omd.Name = "test-mgmd-ext"
	f.expectCreateAction(ns, "", "v1", "services", &corev1.Service{ObjectMeta: *omd})

	omd.Name = "test-ndbd"
	f.expectCreateAction(ns, "", "v1", "services", &corev1.Service{ObjectMeta: *omd})

	omd.Name = "test-mysqld-ext"
	f.expectCreateAction(ns, "", "v1", "services", &corev1.Service{ObjectMeta: *omd})

	omd.Name = "test-pdb"
	f.expectCreateAction(ns, "policy", "v1beta1", "poddisruptionbudgets",
		&policyv1beta1.PodDisruptionBudget{ObjectMeta: *omd})

	omd.Name = "test-config"
	f.expectCreateAction(ns, "", "v1", "configmaps", &corev1.ConfigMap{ObjectMeta: *omd})

	omd.Name = "test-mgmd"
	f.expectCreateAction(ns, "apps", "v1", "statefulsets", &appsv1.StatefulSet{ObjectMeta: *omd})
	omd.Name = "test-ndbd"
	f.expectCreateAction(ns, "apps", "v1", "statefulsets", &appsv1.StatefulSet{ObjectMeta: *omd})

	omd.Name = "test-mysqld"
	f.expectCreateAction(ns, "apps", "v1", "deployments", &appsv1.Deployment{ObjectMeta: *omd})

	//f.expectUpdateNdbStatusAction(ns, ndb)

	f.run(getKey(ndb, t))

	// run again without error
	f.runController(getKey(ndb, t), false)
}

func int32Ptr(i int32) *int32 { return &i }
