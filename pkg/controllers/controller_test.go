// Copyright (c) 2020, 2024, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	"context"
	"reflect"
	"strings"
	"testing"

	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	kubeinformers "k8s.io/client-go/informers"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	core "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"
	klog "k8s.io/klog/v2"

	ndbcontroller "github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"
	"github.com/mysql/ndb-operator/pkg/generated/clientset/versioned/fake"
	informers "github.com/mysql/ndb-operator/pkg/generated/informers/externalversions"
	"github.com/mysql/ndb-operator/pkg/helpers/testutils"
)

type fixture struct {
	t *testing.T

	// Fake clientsets for NdbCluster and other K8s objects
	ndbclient *fake.Clientset
	k8sclient *k8sfake.Clientset

	// Informer factories for NdbCluster and other K8s objects
	ndbIf informers.SharedInformerFactory
	k8sIf kubeinformers.SharedInformerFactory

	// stop channel
	stopCh chan struct{}

	// Actions expected to happen on the client.
	kubeActions []core.Action
	ndbActions  []core.Action

	// Objects to be loaded into the fake clientset and indexers
	k8sObjects []runtime.Object
	ndbObjects []runtime.Object

	c *Controller
}

func newFixture(t *testing.T, ndbclusters ...*ndbcontroller.NdbCluster) *fixture {
	f := &fixture{
		t:          t,
		k8sObjects: []runtime.Object{},
		ndbObjects: []runtime.Object{},
		stopCh:     make(chan struct{}),
	}

	// init the ndbObjects with objects from ndbclusters
	for _, ndb := range ndbclusters {
		f.ndbObjects = append(f.ndbObjects, ndb)
	}

	// Initialize clientsets and InformerFactories
	f.ndbclient = fake.NewSimpleClientset(f.ndbObjects...)
	f.ndbIf = informers.NewSharedInformerFactory(f.ndbclient, 0)

	f.k8sclient = k8sfake.NewSimpleClientset(f.k8sObjects...)
	f.k8sIf = kubeinformers.NewSharedInformerFactory(f.k8sclient, 0)

	return f
}

func (f *fixture) startInformers() {
	f.ndbIf.Start(f.stopCh)
	f.k8sIf.Start(f.stopCh)
}

func (f *fixture) newController() {

	f.c = NewController(f.k8sclient, f.ndbclient, f.k8sIf, f.ndbIf)

	for _, n := range f.ndbObjects {
		if err := f.ndbIf.Mysql().V1().NdbClusters().Informer().GetIndexer().Add(n); err != nil {
			f.t.Fatal("Unexpected error :", err)
		}
	}

	f.startInformers()

	// Wait for the caches to be synced
	if ok := cache.WaitForNamedCacheSync(
		controllerName, f.stopCh, f.c.informerSyncedMethods...); !ok {
		f.t.Fatal("failed to wait for caches to sync")
	}
}

func (f *fixture) close() {
	klog.Info("Closing fixture")
	// close the stop channel to stop the informers
	close(f.stopCh)
}

// runControllerAndValidateActions runs the controller and validate the actions
func (f *fixture) runControllerAndValidateActions(nc *ndbcontroller.NdbCluster, expectError bool, expectedErrors []string) {

	ndbKey := getKey(nc, f.t)

	// run the sync loop once
	sr := f.c.syncHandler(context.TODO(), ndbKey)

	// validate the output
	err := sr.getError()

	if expectError {
		if err == nil {
			// Error expected but there is none
			f.t.Error("expected error syncing ndb, got nil")
		} else if expectedErrors != nil {
			// Only the errors in expectedErrors are allowed
			errorOk := false
			for _, expectedErr := range expectedErrors {
				if strings.Contains(err.Error(), expectedErr) {
					f.t.Logf("Expected error and received one (good): %s", err)
					errorOk = true
					break
				}
			}
			if !errorOk {
				// Unexpected error occurred
				f.t.Errorf("Recieved unexpected error : %s", err)
			}
		} else {
			// Any errors are accepted as expectedErrors is nil
			f.t.Logf("Expected error and received one (good): %s", err)
		}
	} else {
		// NO error is expected
		if err == nil {
			f.t.Logf("Successfully completed one reconciliation loop")
		} else {
			f.t.Errorf("Recieved unexpected error : %s", err)
		}
	}

	// validate the actions
	f.checkActions()
}

func (f *fixture) checkActions() {

	actions := filterInformerActions(f.ndbclient.Actions())
	k8sActions := filterInformerActions(f.k8sclient.Actions())

	for i, action := range actions {

		if len(f.ndbActions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(actions)-len(f.ndbActions), actions[i:])
			break
		}

		expectedAction := f.ndbActions[i]

		/*
			s, _ := json.Marshal(expectedAction)
			fmt.Printf("[%d] %d : %s\n", i, len(f.ndbActions), s)
			s, _ = json.Marshal(action)
			fmt.Printf("[%d] %d : %s\n\n", i, len(f.ndbActions), s)
		*/

		checkAction(expectedAction, action, f.t)
	}

	if len(f.ndbActions) > len(actions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.ndbActions)-len(actions), f.ndbActions[len(actions):])
	}

	for i, action := range k8sActions {

		if len(f.kubeActions) < i+1 {
			f.t.Errorf("%d unexpected actions: %+v", len(k8sActions)-len(f.kubeActions), k8sActions[i:])
			break
		}

		expectedAction := f.kubeActions[i]
		checkAction(expectedAction, action, f.t)
		/*
			s, _ := json.Marshal(expectedAction)
			fmt.Printf("[%d] %d : %s\n", i, len(f.kubeActions), s)
			s, _ = json.Marshal(action)
			fmt.Printf("[%d] %d : %s\n\n", i, len(f.kubeActions), s)
		*/
	}

	if len(f.kubeActions) > len(k8sActions) {
		f.t.Errorf("%d additional expected actions:%+v", len(f.kubeActions)-len(k8sActions), f.kubeActions[len(k8sActions):])
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
		expOM = extO.(*policyv1.PodDisruptionBudget).ObjectMeta
		actOM = actO.(*policyv1.PodDisruptionBudget).ObjectMeta
	case "services":
		expOM = extO.(*corev1.Service).ObjectMeta
		actOM = actO.(*corev1.Service).ObjectMeta
	case "secrets":
		expOM = extO.(*corev1.Secret).ObjectMeta
		actOM = actO.(*corev1.Secret).ObjectMeta
	case "serviceaccounts":
		expOM = extO.(*corev1.ServiceAccount).ObjectMeta
		actOM = actO.(*corev1.ServiceAccount).ObjectMeta
	case "ndbclusters":
		expOM = extO.(*ndbcontroller.NdbCluster).ObjectMeta
		actOM = actO.(*ndbcontroller.NdbCluster).ObjectMeta
	case "validatingwebhookconfigurations":
		expOM = extO.(*admissionregistrationv1.ValidatingWebhookConfiguration).ObjectMeta
		actOM = actO.(*admissionregistrationv1.ValidatingWebhookConfiguration).ObjectMeta
	default:
		t.Errorf("Action has unkown type. Got: %s", actual.GetResource().Resource)
	}

	return expOM, actOM
}

// checkAction verifies that expected and actual actions are equal and both have
// same attached resources
func checkAction(expected, actual core.Action, t *testing.T) {
	// TODO: Compare the complete GroupVersionResource rather than just Resource
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
			t.Errorf("Action %s %s has wrong name %s. Expected : %s",
				a.GetVerb(), a.GetResource().Resource, actOM.Name, expOM.Name)
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
			t.Errorf("Action %s %s has wrong owner reference. \nExpected : %#v\nActual : %#v\n",
				a.GetVerb(), a.GetResource().Resource, expOM.OwnerReferences, actOM.OwnerReferences)
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
		// Do nothing. Just verifying that there was an update is enough.

	case core.PatchActionImpl:
		e, _ := expected.(core.PatchActionImpl)

		expPatch := e.GetPatch()
		if expPatch == nil {
			// Skip comparing the patch if the expected patch is nil
			t.Logf("Skipped comparing patches as expected patch is empty")
			return
		}

		patch := a.GetPatch()
		if !reflect.DeepEqual(expPatch, patch) {
			t.Errorf("Action %s %s has wrong patch\n",
				a.GetVerb(), a.GetResource().Resource)
			//				t.Errorf("Action %s %s has wrong patch\nDiff:\n %s",
			//				a.GetVerb(), a.GetResource().Resource, diff.ObjectGoPrintSideBySide(expPatch, patch))
		}

	case core.DeleteActionImpl:
		e, _ := expected.(core.DeleteActionImpl)
		if e.Name != a.Name {
			t.Errorf("Expected delete action on object %q but actual delete was on object %q", e.Name, a.Name)
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
	//klog.Infof("Filtering %d ndbActions", len(ndbActions))
	ret := []core.Action{}
	for _, action := range actions {
		// Ignore all gets used by the controllers
		if (action.GetNamespace() == "default" &&
			(action.Matches("get", "secrets") ||
				action.Matches("get", "ndbclusters"))) ||
			(action.GetNamespace() == "" && action.Matches("get", "version")) {
			//klog.Infof("Filtering +%v", action)
			continue
		}
		// Ignore all list and watches mostly called by the informers
		if len(action.GetNamespace()) == 0 &&
			(action.Matches("list", "ndbclusters") ||
				action.Matches("watch", "ndbclusters") ||
				action.Matches("list", "pods") ||
				action.Matches("watch", "pods") ||
				action.Matches("list", "configmaps") ||
				action.Matches("watch", "configmaps") ||
				action.Matches("list", "services") ||
				action.Matches("watch", "services") ||
				action.Matches("list", "serviceaccounts") ||
				action.Matches("watch", "serviceaccounts") ||
				action.Matches("list", "persistentvolumeclaims") ||
				action.Matches("watch", "persistentvolumeclaims") ||
				action.Matches("list", "poddisruptionbudgets") ||
				action.Matches("watch", "poddisruptionbudgets") ||
				action.Matches("list", "statefulsets") ||
				action.Matches("watch", "statefulsets") ||
				action.Matches("list", "secrets") ||
				action.Matches("watch", "secrets") ||
				action.Matches("list", "validatingwebhookconfigurations")) {
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
	f.kubeActions = append(f.kubeActions, core.NewCreateAction(grpVersionResource, ns, o))
}

// expectPatchAction adds an expected patch action.
// checkAction will validate the patch action and compare sent patch with the expPatch.
// To skip the patch comparison, pass nil for expPatch
func (f *fixture) expectPatchAction(ns string, resource string, name string, pt types.PatchType, expPatch []byte) {
	f.kubeActions = append(f.kubeActions,
		core.NewPatchAction(schema.GroupVersionResource{Resource: resource}, ns, name, pt, expPatch))
}

func (f *fixture) expectDeleteAction(ns, group, version, resource, name string) {
	grpVersionResource := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	f.kubeActions = append(f.kubeActions, core.NewDeleteAction(grpVersionResource, ns, name))
}

func (f *fixture) expectNdbClusterStatusUpdateAction(ns string, group, version, resource string) {
	grpVersionResource := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
	f.ndbActions = append(f.ndbActions, core.NewUpdateSubresourceAction(grpVersionResource, "status", ns, nil))
}

func getKey(nc *ndbcontroller.NdbCluster, t *testing.T) string {
	key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(nc)
	if err != nil {
		t.Errorf("Unexpected error getting key for foo %q: %v", getNamespacedName(nc), err)
		return ""
	}
	return key
}

func getObjectMetadata(name string, ndb *ndbcontroller.NdbCluster) *metav1.ObjectMeta {

	gvk := schema.GroupVersionKind{
		Group:   ndbcontroller.SchemeGroupVersion.Group,
		Version: ndbcontroller.SchemeGroupVersion.Version,
		Kind:    "NdbCluster",
	}

	return &metav1.ObjectMeta{
		Labels: ndb.GetLabels(),
		Name:   name,
		OwnerReferences: []metav1.OwnerReference{
			*metav1.NewControllerRef(ndb, gvk),
		},
	}
}

// Channel to wait for the StatefulSets to become ready
var sfsetReady = make(chan bool)

func markStatefulSetAsReadyOnAdd(f *fixture) {
	f.k8sIf.Apps().V1().StatefulSets().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			sfset := obj.(*appsv1.StatefulSet)
			// Immediately mark the StatefulSet as ready once it is added
			sfset.Status.ReadyReplicas = *sfset.Spec.Replicas
			sfset.Status.UpdatedReplicas = *sfset.Spec.Replicas
			sfset.Status.Replicas = *sfset.Spec.Replicas
			sfset.Status.CurrentReplicas = *sfset.Spec.Replicas
			// Send a signal to the sfsetReady channel to denote the sfset readiness
			sfsetReady <- true
		},
	})
}

func TestCreatesCluster(t *testing.T) {

	ns := metav1.NamespaceDefault
	ndb := testutils.NewTestNdb(ns, "test", 2)

	f := newFixture(t, ndb)
	defer f.close()
	// create new controller
	f.newController()

	// Register handler to make StatefulSets ready as soon as they are created
	markStatefulSetAsReadyOnAdd(f)

	// Expect actions for first loop
	// One configmap for NdbCluster resource
	omd := getObjectMetadata("test-config", ndb)
	f.expectCreateAction(ns, "", "v1", "configmaps", &corev1.ConfigMap{ObjectMeta: *omd})

	// Secret for the NDB operator user password
	omd.Name = "test-ndb-operator-password"
	f.expectCreateAction(ns, "", "v1", "secrets", &corev1.Secret{ObjectMeta: *omd})

	// one service for mgmd
	omd.Name = "test-mgmd"
	f.expectCreateAction(ns, "", "v1", "services", &corev1.Service{ObjectMeta: *omd})

	// One StatefulSet for management nodes
	omd.Name = "test-sa"
	f.expectCreateAction(ns, "", "v1", "serviceaccounts", &corev1.ServiceAccount{ObjectMeta: *omd})

	// One StatefulSet for management nodes
	omd.Name = "test-mgmd"
	f.expectCreateAction(ns, "apps", "v1", "statefulsets", &appsv1.StatefulSet{ObjectMeta: *omd})

	// Expect an update on ndbcluster/status
	f.expectNdbClusterStatusUpdateAction(ns, "mysql.oracle.com", "v1", "ndbclusters")

	// The reconciliation loop ends here. It continues only after the management nodes are ready.
	f.runControllerAndValidateActions(ndb, false, nil)

	// Wait for mgmd sfset to become ready
	<-sfsetReady

	// Expect Actions for the next loop
	// one headless service for data nodes
	omd.Name = "test-ndbmtd"
	f.expectCreateAction(ns, "", "v1", "services", &corev1.Service{ObjectMeta: *omd})

	// One StatefulSet for Data nodes
	omd.Name = "test-ndbmtd"
	f.expectCreateAction(ns, "apps", "v1", "statefulsets", &appsv1.StatefulSet{ObjectMeta: *omd})

	// Expect an update on ndbcluster/status
	f.expectNdbClusterStatusUpdateAction(ns, "mysql.oracle.com", "v1", "ndbclusters")

	// The reconciliation loop ends here. It continues only after the data nodes are ready.
	f.runControllerAndValidateActions(ndb, false, nil)

	// Wait for ndbmtd sfset to become ready
	<-sfsetReady

	// Expect Actions for the next loop
	// Secret for the MySQL Server root password
	omd.Name = "test-mysqld-root-password"
	f.expectCreateAction(ns, "", "v1", "secrets", &corev1.Secret{ObjectMeta: *omd})

	// Governing Service
	omd.Name = "test-mysqld"
	f.expectCreateAction(ns, "", "v1", "services", &corev1.Service{ObjectMeta: *omd})

	// One StatefulSet for MySQL Servers
	omd.Name = "test-mysqld"
	f.expectCreateAction(ns, "apps", "v1", "statefulsets", &appsv1.StatefulSet{ObjectMeta: *omd})

	// Expect an update on ndbcluster/status
	f.expectNdbClusterStatusUpdateAction(ns, "mysql.oracle.com", "v1", "ndbclusters")

	// The reconciliation loop ends here.
	f.runControllerAndValidateActions(ndb, false, nil)
}
