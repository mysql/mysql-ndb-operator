// Copyright (c) 2021, 2023, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package controllers

import (
	eventsv1 "k8s.io/api/events/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/events"
	klog "k8s.io/klog/v2"

	ndbscheme "github.com/mysql/ndb-operator/pkg/generated/clientset/versioned/scheme"
)

const (
	// ReasonResourceExists is the reason used for an Event when the
	// operator fails to sync the Ndb object with MySQL Cluster due to
	// some resource of the same name already existing but not owned by
	// the Ndb object.
	ReasonResourceExists = "ResourceExists"
	// ReasonSyncSuccess is the reason used for an Event when
	// the MySQL Cluster is successfully synced with the Ndb object.
	ReasonSyncSuccess = "SyncSuccess"
	// ReasonInSync is the reason used for an Event when the MySQL Cluster
	// is already in sync with the spec of the Ndb object.
	ReasonInSync = "InSync"

	// ActionNone is the action used for an Event when the operator does nothing.
	ActionNone = "None"
	// ActionSynced is the action used for an Event when the operator
	// makes changes to the MySQL Cluster and successfully syncs it with
	// the Ndb object.
	ActionSynced = "Synced"

	// MessageResourceExists is the message used for an Event when the
	// operator fails to sync the Ndb object with MySQL Cluster due to
	// some resource of the same name already existing but not owned by
	// the Ndb object.
	MessageResourceExists = "Resource %q already exists and is not managed by Ndb"
	// MessageSyncSuccess is the message used for an Event when
	// the MySQL Cluster is successfully synced with the Ndb object.
	MessageSyncSuccess = "MySQL Cluster was successfully synced up to match the spec"
	// MessageInSync is the message used for an Event when the MySQL Cluster
	// is already in sync with the spec of the Ndb object.
	MessageInSync = "MySQL Cluster is in sync with the Ndb object"
)

// reporting controller for the events
const controllerName = "ndb-controller"

// newEventRecorder initialises an event broadcaster
// and then creates a new recorder to send events to it
func newEventRecorder(clientset kubernetes.Interface) events.EventRecorder {
	// Add ndb-controller types to the default Kubernetes Scheme
	// so Events can be logged for ndb-controller types.
	utilruntime.Must(ndbscheme.AddToScheme(scheme.Scheme))

	// Create event broadcaster
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := events.NewBroadcaster(
		&events.EventSinkImpl{
			Interface: clientset.EventsV1(),
		})
	eventBroadcaster.StartRecordingToSink(make(chan struct{}))

	// Add event handler to the broadcaster to log events on receiving them
	eventBroadcaster.StartEventWatcher(func(event runtime.Object) {
		e := event.(*eventsv1.Event)
		klog.Infof("Event(%#v): type: '%s' action: '%s' reason: '%s' %s",
			e.Regarding, e.Type, e.Action, e.Reason, e.Note)
	})

	// Create a new recorder to send events to the broadcaster
	return eventBroadcaster.NewRecorder(scheme.Scheme, controllerName)
}
