package event_utils

import (
	"context"
	"errors"
	"time"

	v1 "k8s.io/api/events/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/kubernetes/test/e2e/framework"
)

const (
	defaultWaitTimeout = 10 * time.Minute
)

// GetLastKnownEventResourceVersion returns the ResourceVersion
// of the latest event sent by the ndb-controller
func GetLastKnownEventResourceVersion(clientset clientset.Interface, namespace string) string {
	ctx, cancel := context.WithTimeout(context.Background(), defaultWaitTimeout)
	defer cancel()

	// list all the events to get the latest
	eventList, err := clientset.EventsV1().Events(namespace).List(ctx, metav1.ListOptions{
		FieldSelector: fields.Set{
			"reportingController": "ndb-controller",
		}.String(),
	})
	framework.ExpectNoError(err, "failed to list all events")

	numOfEvents := len(eventList.Items)
	if numOfEvents == 0 {
		// no events exist
		return ""
	}

	// return the ResourceVersion of the latest event
	return eventList.Items[numOfEvents-1].ResourceVersion
}

// WaitForEvent waits for an expected event to occur
func WaitForEvent(clientset clientset.Interface, namespace, eventReason string,
	lastKnownResourceVersion string, waitTimeout ...time.Duration) error {
	// create a context with wait timeout
	if len(waitTimeout) > 1 {
		panic("Wrong usage of WaitForEvent method")
	}
	timeout := defaultWaitTimeout
	if len(waitTimeout) == 1 {
		timeout = waitTimeout[0]
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// create the event watch interface to watch for events from ndb controller
	watcher, err := clientset.EventsV1().Events(namespace).Watch(ctx, metav1.ListOptions{
		FieldSelector: fields.Set{
			"reportingController": "ndb-controller",
		}.String(),
		ResourceVersion: lastKnownResourceVersion,
	})
	if err != nil {
		return err
	}

	// Listen to all events and wait for an event with eventReason to occur
	for {
		select {
		case watchEvent := <-watcher.ResultChan():
			event := watchEvent.Object.(*v1.Event)
			if event.Reason == eventReason {
				return nil
			}
		case <-ctx.Done():
			// wait timeout
			return errors.New("waitForEvent timed out")
		}
	}
}
