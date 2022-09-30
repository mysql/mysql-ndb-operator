// Copyright (c) 2022, Oracle and/or its affiliates.
//
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl/

package statefulset

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/mysql/ndb-operator/pkg/apis/ndbcontroller/v1"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func errorIfNotEqual(t *testing.T, actual interface{}, expectedJsonStr, desc string) {
	// Convert the actual and expected values into json objects and compare them
	actualJsonStr, err := json.Marshal(actual)
	if err != nil {
		t.Errorf("json.Marshal failed for testcase %q : %s", desc, err.Error())
		return
	}

	var actualInterface interface{}
	err = json.Unmarshal(actualJsonStr, &actualInterface)
	if err != nil {
		t.Errorf("json.UnMarshal failed for testcase %q : %s", desc, err.Error())
		return
	}

	var expectedInterface interface{}
	err = json.Unmarshal([]byte(expectedJsonStr), &expectedInterface)
	if err != nil {
		t.Errorf("json.UnMarshal failed for testcase %q : %s", desc, err.Error())
		return
	}

	if !reflect.DeepEqual(actualInterface, expectedInterface) {
		t.Errorf("Testcase %q failed, Expected : %s, Actual : %s", desc, expectedJsonStr, actualJsonStr)
	}
}

func Test_setPodSpecFromNdbPodSpec_Resources(t *testing.T) {
	for _, tc := range []struct {
		ndbPodSpec                *v1.NdbClusterPodSpec
		defaultResources          *corev1.ResourceRequirements
		desc                      string
		expectedResourceInPodSpec string
	}{
		{
			ndbPodSpec: &v1.NdbClusterPodSpec{
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("500Gi"),
					},
				},
			},
			desc:                      "ndbPodSpec has Resource Limits",
			expectedResourceInPodSpec: `{"limits": {"memory": "500Gi"}}`,
		},
		{
			ndbPodSpec: &v1.NdbClusterPodSpec{
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("500Gi"),
					},
				},
			},
			desc:                      "ndbPodSpec has Resource Requests",
			expectedResourceInPodSpec: `{"requests":{"memory": "500Gi"}}`,
		},
		{
			defaultResources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("200Gi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("100Gi"),
				},
			},
			desc:                      "ndbPodSpec has default Resources and Limits",
			expectedResourceInPodSpec: `{"limits": {"memory": "200Gi"}, "requests": {"memory": "100Gi"}}`,
		},
		{
			ndbPodSpec: &v1.NdbClusterPodSpec{
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("500Gi"),
					},
				},
			},
			defaultResources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("200Gi"),
				},
			},
			desc:                      "ndbPodSpec has Resource Limits and pod has default Limits",
			expectedResourceInPodSpec: `{"limits": {"memory": "500Gi"}}`,
		},
		{
			ndbPodSpec: &v1.NdbClusterPodSpec{
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("500Gi"),
					},
				},
			},
			defaultResources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("200Gi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("100Gi"),
				},
			},
			desc:                      "ndbPodSpec has Resource Limits and pod has default Requests and Limits",
			expectedResourceInPodSpec: `{"limits": {"memory": "500Gi"}, "requests": {"memory": "100Gi"}}`,
		},
		{
			ndbPodSpec: &v1.NdbClusterPodSpec{
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("500Gi"),
					},
				},
			},
			defaultResources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("200Gi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("100Gi"),
				},
			},
			desc:                      "ndbPodSpec has Resource Limits and pod has default Requests and Limits",
			expectedResourceInPodSpec: `{"limits": {"memory": "200Gi"}, "requests": {"memory": "500Gi"}}`,
		},
		{
			ndbPodSpec: &v1.NdbClusterPodSpec{
				Resources: &corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("10Gi"),
					},
				},
			},
			defaultResources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("200Gi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("100Gi"),
				},
			},
			desc: "ndbPodSpec has Resource Limits for Storage and pod has default Requests and Limits for Memory",
			// Requests should be merged
			expectedResourceInPodSpec: `{"limits":{"memory":"200Gi"},"requests":{"memory":"100Gi","storage":"10Gi"}}`,
		},
	} {
		// For every testcase create a podSpec with dummyLimits
		var podSpec corev1.PodSpec
		podSpec.Containers = []corev1.Container{
			{
				Name:  "test-container",
				Image: "test-image",
			},
		}

		if tc.defaultResources != nil {
			tc.defaultResources.DeepCopyInto(&podSpec.Containers[0].Resources)
		}

		// Set and verify if the values are set properly
		CopyPodSpecFromNdbPodSpec(&podSpec, tc.ndbPodSpec)
		errorIfNotEqual(t, podSpec.Containers[0].Resources, tc.expectedResourceInPodSpec, tc.desc)
	}
}

func Test_setPodSpecFromNdbPodSpec_Affinity(t *testing.T) {
	for _, tc := range []struct {
		ndbPodSpec                *v1.NdbClusterPodSpec
		defaultAffinity           *corev1.Affinity
		desc                      string
		expectedAffinityInPodSpec string
	}{
		{
			ndbPodSpec: &v1.NdbClusterPodSpec{
				Affinity: &corev1.Affinity{
					PodAntiAffinity: &corev1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"key1": "value1",
									},
								},
								TopologyKey: "hostname",
							},
						},
					},
				},
			},
			desc: "ndbPodSpec has PodAntiAffinity",
			expectedAffinityInPodSpec: `{"podAntiAffinity":
											{"requiredDuringSchedulingIgnoredDuringExecution":
												[{"labelSelector":{"matchLabels":{"key1":"value1"}},"topologyKey":"hostname"}]}}`,
		},
		{
			ndbPodSpec: &v1.NdbClusterPodSpec{
				Affinity: &corev1.Affinity{
					PodAntiAffinity: &corev1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"key1": "value1",
									},
								},
								TopologyKey: "hostname",
							},
						},
					},
				},
			},
			defaultAffinity: &corev1.Affinity{
				PodAffinity: &corev1.PodAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"key2": "value2",
								},
							},
							TopologyKey: "hostname",
						},
					},
				},
			},
			desc: "ndbPodSpec has PodAntiAffinity and podSpec has default PodAffinity",
			// The default should be preserved as the nodeSpec doesn't have a PodAffinity specified
			expectedAffinityInPodSpec: `{"podAffinity":
											{"requiredDuringSchedulingIgnoredDuringExecution":
												[{"labelSelector":{"matchLabels":{"key2":"value2"}},"topologyKey":"hostname"}]},
                                         "podAntiAffinity":
											{"requiredDuringSchedulingIgnoredDuringExecution":
												[{"labelSelector":{"matchLabels":{"key1":"value1"}},"topologyKey":"hostname"}]}}`,
		},
		{
			ndbPodSpec: &v1.NdbClusterPodSpec{
				Affinity: &corev1.Affinity{
					PodAffinity: &corev1.PodAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"key1": "value1",
									},
								},
								TopologyKey: "hostname",
							},
						},
					},
					PodAntiAffinity: &corev1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"key2": "value2",
									},
								},
								TopologyKey: "hostname",
							},
						},
					},
				},
			},
			defaultAffinity: &corev1.Affinity{
				PodAffinity: &corev1.PodAffinity{
					RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
						{
							LabelSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"key3": "value3",
								},
							},
							TopologyKey: "hostname",
						},
					},
				},
			},
			desc: "ndbPodSpec has PodAffinity and PodAntiAffinity, and podSpec has default PodAffinity",
			// The default should be overridden as the nodeSpec has a PodAffinity specified
			expectedAffinityInPodSpec: `{"podAffinity":
											{"requiredDuringSchedulingIgnoredDuringExecution":
												[{"labelSelector":{"matchLabels":{"key1":"value1"}},"topologyKey":"hostname"}]},
                                         "podAntiAffinity":
											{"requiredDuringSchedulingIgnoredDuringExecution":
												[{"labelSelector":{"matchLabels":{"key2":"value2"}},"topologyKey":"hostname"}]}}`,
		},
		{
			ndbPodSpec: &v1.NdbClusterPodSpec{
				Affinity: &corev1.Affinity{
					NodeAffinity: &corev1.NodeAffinity{
						PreferredDuringSchedulingIgnoredDuringExecution: []corev1.PreferredSchedulingTerm{
							{
								Weight: 10,
								Preference: corev1.NodeSelectorTerm{
									MatchExpressions: []corev1.NodeSelectorRequirement{
										{
											Key:      "mysql-cluster",
											Operator: corev1.NodeSelectorOpExists,
										},
									},
								},
							},
						},
					},
					PodAffinity: &corev1.PodAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"key1": "value1",
									},
								},
								TopologyKey: "hostname",
							},
						},
					},
					PodAntiAffinity: &corev1.PodAntiAffinity{
						RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
							{
								LabelSelector: &metav1.LabelSelector{
									MatchLabels: map[string]string{
										"key2": "value2",
									},
								},
								TopologyKey: "hostname",
							},
						},
					},
				},
			},
			desc: "ndbPodSpec has all 3 Affinities",
			expectedAffinityInPodSpec: `{"nodeAffinity":
											{"preferredDuringSchedulingIgnoredDuringExecution":
												[{"weight":10,"preference":{"matchExpressions":[{"key":"mysql-cluster","operator":"Exists"}]}}]},
										 "podAffinity":
											{"requiredDuringSchedulingIgnoredDuringExecution":
												[{"labelSelector":{"matchLabels":{"key1":"value1"}},"topologyKey":"hostname"}]},
										 "podAntiAffinity":
											{"requiredDuringSchedulingIgnoredDuringExecution":
												[{"labelSelector":{"matchLabels":{"key2":"value2"}},"topologyKey":"hostname"}]}}`,
		},
	} {
		// For every testcase create a podSpec with dummyLimits
		var podSpec corev1.PodSpec

		// Set default Affinity in podSpec (if any)
		if tc.defaultAffinity != nil {
			podSpec.Affinity = new(corev1.Affinity)
			tc.defaultAffinity.DeepCopyInto(podSpec.Affinity)
		}

		// Set and verify if the values are set properly
		CopyPodSpecFromNdbPodSpec(&podSpec, tc.ndbPodSpec)
		errorIfNotEqual(t, podSpec.Affinity, tc.expectedAffinityInPodSpec, tc.desc)
	}
}

func Test_setPodSpecFromNdbPodSpec_Misc(t *testing.T) {
	tolerationSeconds := int64(42)
	ndbPodSpec := &v1.NdbClusterPodSpec{
		NodeSelector: map[string]string{
			"node-reserved-for":     "ndbd",
			"node-not-reserved-for": "mysqld",
		},
		SchedulerName: "custom-scheduler",
		Tolerations: []corev1.Toleration{
			{
				Key:               "Key1",
				Operator:          corev1.TolerationOpEqual,
				Value:             "Value1",
				Effect:            corev1.TaintEffectNoSchedule,
				TolerationSeconds: &tolerationSeconds,
			},
		},
	}
	expectedNodeSelectorInPodSpec := `{"node-reserved-for":"ndbd", "node-not-reserved-for":"mysqld"}`
	expectedTolerations := `[{"key":"Key1","operator":"Equal","value":"Value1","effect":"NoSchedule","tolerationSeconds":42}]`

	var podSpec corev1.PodSpec

	// Set and verify if the values are set properly
	CopyPodSpecFromNdbPodSpec(&podSpec, ndbPodSpec)
	errorIfNotEqual(t, podSpec.NodeSelector, expectedNodeSelectorInPodSpec, "NodeSelector")
	errorIfNotEqual(t, podSpec.Tolerations, expectedTolerations, "Tolerations")
	if podSpec.SchedulerName != ndbPodSpec.SchedulerName {
		t.Errorf("Unexpected value in Scheduler name. Expected : %q, Actual %q", ndbPodSpec.SchedulerName, podSpec.SchedulerName)
	}
}
