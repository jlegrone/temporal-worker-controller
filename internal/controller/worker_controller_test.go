// Unless explicitly stated otherwise all files in this repository are licensed under the MIT License.
//
// This product includes software developed at Datadog (https://www.datadoghq.com/). Copyright 2024 Datadog, Inc.

package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	temporaliov1alpha1 "github.com/DataDog/temporal-worker-controller/api/v1alpha1"
)

var (
	testPodTemplate = v1.PodTemplateSpec{
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "main",
					Image: "foo/bar@sha256:deadbeef",
				},
			},
		},
	}
)

func newTestWorkerSpec(replicas int32) temporaliov1alpha1.TemporalWorker {
	return temporaliov1alpha1.TemporalWorker{
		Spec: temporaliov1alpha1.TemporalWorkerSpec{
			Replicas: &replicas,
			Template: testPodTemplate,
			WorkerOptions: temporaliov1alpha1.WorkerOptions{
				TemporalNamespace: "baz",
				TaskQueue:         "qux",
			},
		},
	}
}

func newTestDeployment(podSpec v1.PodTemplateSpec, desiredReplicas int32) *appsv1.Deployment {
	return &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "foo",
			Name:      "bar-7476c6b88c",
			Annotations: map[string]string{
				"temporal.io/build-id": "7476c6b88c",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &desiredReplicas,
			Template: podSpec,
		},
	}
}

func newTestVersionSet(reachabilityStatus temporaliov1alpha1.ReachabilityStatus, deploymentName string) *temporaliov1alpha1.CompatibleVersionSet {
	result := temporaliov1alpha1.CompatibleVersionSet{
		ReachabilityStatus: reachabilityStatus,
		InactiveBuildIDs:   nil,
		DefaultBuildID:     "test-id",
		DeployedBuildID:    "test-id",
	}

	if deploymentName != "" {
		panic("todo")
		//result.Deployment = &v1.ObjectReference{
		//	Namespace: "foo",
		//	Name:      deploymentName,
		//}
	}

	return &result
}

func TestGeneratePlan(t *testing.T) {
	type testCase struct {
		observedState temporaliov1alpha1.TemporalWorkerStatus
		desiredState  temporaliov1alpha1.TemporalWorker
		expectedPlan  plan
	}

	testCases := map[string]testCase{
		"no action needed": {
			observedState: temporaliov1alpha1.TemporalWorkerStatus{
				DefaultVersionSet: newTestVersionSet(temporaliov1alpha1.ReachabilityStatusActive, "foo-a"),
			},
			desiredState: newTestWorkerSpec(3),
			expectedPlan: plan{
				DeleteDeployments:      nil,
				CreateDeployment:       nil,
				RegisterDefaultVersion: "",
			},
		},
		"create deployment": {
			observedState: temporaliov1alpha1.TemporalWorkerStatus{
				DefaultVersionSet: &temporaliov1alpha1.CompatibleVersionSet{
					ReachabilityStatus: temporaliov1alpha1.ReachabilityStatusActive,
					InactiveBuildIDs:   nil,
					DefaultBuildID:     "a",
				},
				DeprecatedVersionSets: nil,
			},
			desiredState: newTestWorkerSpec(3),
			expectedPlan: plan{
				DeleteDeployments:      nil,
				CreateDeployment:       newTestDeployment(testPodTemplate, 3),
				RegisterDefaultVersion: "",
			},
		},
		"delete unreachable deployments": {
			observedState: temporaliov1alpha1.TemporalWorkerStatus{
				DefaultVersionSet: newTestVersionSet(temporaliov1alpha1.ReachabilityStatusActive, "foo-a"),
				DeprecatedVersionSets: []*temporaliov1alpha1.CompatibleVersionSet{
					newTestVersionSet(temporaliov1alpha1.ReachabilityStatusUnreachable, "foo-b"),
					newTestVersionSet(temporaliov1alpha1.ReachabilityStatusActive, "foo-c"),
					newTestVersionSet(temporaliov1alpha1.ReachabilityStatusUnreachable, "foo-d"),
				},
			},
			desiredState: newTestWorkerSpec(3),
			expectedPlan: plan{
				DeleteDeployments: []*appsv1.Deployment{
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "foo",
							Name:      "foo-b",
						},
					},
					{
						ObjectMeta: metav1.ObjectMeta{
							Namespace: "foo",
							Name:      "foo-d",
						},
					},
				},
				CreateDeployment:       nil,
				RegisterDefaultVersion: "",
			},
		},
	}

	r := &TemporalWorkerReconciler{}

	for name, tc := range testCases {
		t.Run(name, func(t *testing.T) {
			actualPlan, err := r.generatePlan(context.Background(), tc.observedState, tc.desiredState)
			assert.NoError(t, err)
			assert.Equal(t, &tc.expectedPlan, actualPlan)
		})
	}
}
