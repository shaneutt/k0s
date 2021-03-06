/*
Copyright 2020 k0s authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/
package server

import (
	"context"
	"testing"

	"github.com/k0sproject/k0s/pkg/apis/v1beta1"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/fake"
)

type fakeAlwaysLeaderElector struct {
}

func (f *fakeAlwaysLeaderElector) Run() error     { return nil }
func (f *fakeAlwaysLeaderElector) Init() error    { return nil }
func (f *fakeAlwaysLeaderElector) Stop() error    { return nil }
func (f *fakeAlwaysLeaderElector) Healthy() error { return nil }

func (f *fakeAlwaysLeaderElector) IsLeader() bool {
	return true
}

type fakeNeverLeaderElector struct {
}

func (f *fakeNeverLeaderElector) Run() error     { return nil }
func (f *fakeNeverLeaderElector) Init() error    { return nil }
func (f *fakeNeverLeaderElector) Stop() error    { return nil }
func (f *fakeNeverLeaderElector) Healthy() error { return nil }

func (f *fakeNeverLeaderElector) IsLeader() bool {
	return false
}

var expectedAddresses = []string{
	"185.199.108.153",
	"185.199.109.153",
	"185.199.110.153",
	"185.199.111.153",
}

type fakeClientFactory struct {
	fakeClient kubernetes.Interface
}

func (f *fakeClientFactory) Create() (kubernetes.Interface, error) {
	return f.fakeClient, nil
}

func TestBasicReconcilerWithNoLeader(t *testing.T) {
	var fakeFactory = &fakeClientFactory{
		fakeClient: fake.NewSimpleClientset(),
	}
	config := &v1beta1.ClusterConfig{
		Spec: &v1beta1.ClusterSpec{
			API: &v1beta1.APISpec{
				Address:         "1.2.3.4",
				ExternalAddress: "get.k0s.sh",
			},
		},
	}

	r := NewEndpointReconciler(config, &fakeNeverLeaderElector{}, fakeFactory)

	assert.NoError(t, r.Init())

	assert.NoError(t, r.reconcileEndpoints())
	client, err := fakeFactory.Create()
	assert.NoError(t, err)
	_, err = client.CoreV1().Endpoints("default").Get(context.TODO(), "kubernetes", v1.GetOptions{})
	// The reconciler should not make any modification as we're not the leader so the endpoint should not get created
	assert.Error(t, err)
	assert.True(t, errors.IsNotFound(err))
	//verifyEndpointAddresses(t, expectedAddresses, fakeFactory)
}

func TestBasicReconcilerWithNoExistingEndpoint(t *testing.T) {
	var fakeFactory = &fakeClientFactory{
		fakeClient: fake.NewSimpleClientset(),
	}
	config := &v1beta1.ClusterConfig{
		Spec: &v1beta1.ClusterSpec{
			API: &v1beta1.APISpec{
				Address:         "1.2.3.4",
				ExternalAddress: "get.k0s.sh",
			},
		},
	}

	r := NewEndpointReconciler(config, &fakeAlwaysLeaderElector{}, fakeFactory)

	assert.NoError(t, r.Init())

	assert.NoError(t, r.reconcileEndpoints())
	verifyEndpointAddresses(t, expectedAddresses, fakeFactory)
}

func TestBasicReconcilerWithEmptyEndpointSubset(t *testing.T) {
	var fakeFactory = &fakeClientFactory{
		fakeClient: fake.NewSimpleClientset(),
	}
	existingEp := corev1.Endpoints{
		TypeMeta: v1.TypeMeta{
			Kind:       "Endpoints",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: "kubernetes",
		},
		Subsets: []corev1.EndpointSubset{},
	}
	fakeClient, err := fakeFactory.Create()
	assert.NoError(t, err)
	_, err = fakeClient.CoreV1().Endpoints("default").Create(context.TODO(), &existingEp, v1.CreateOptions{})
	assert.NoError(t, err)
	config := &v1beta1.ClusterConfig{
		Spec: &v1beta1.ClusterSpec{
			API: &v1beta1.APISpec{
				Address:         "1.2.3.4",
				ExternalAddress: "get.k0s.sh",
			},
		},
	}

	r := NewEndpointReconciler(config, &fakeAlwaysLeaderElector{}, fakeFactory)

	assert.NoError(t, r.Init())

	assert.NoError(t, r.reconcileEndpoints())
	verifyEndpointAddresses(t, expectedAddresses, fakeFactory)
}

func TestReconcilerWithNoNeedForUpdate(t *testing.T) {
	var fakeFactory = &fakeClientFactory{
		fakeClient: fake.NewSimpleClientset(),
	}
	existingEp := corev1.Endpoints{
		TypeMeta: v1.TypeMeta{
			Kind:       "Endpoints",
			APIVersion: "v1",
		},
		ObjectMeta: v1.ObjectMeta{
			Name: "kubernetes",
			Annotations: map[string]string{
				"foo": "bar",
			},
		},
		Subsets: []corev1.EndpointSubset{
			{
				Addresses: stringsToEndpointAddresses(expectedAddresses),
			},
		},
	}

	fakeClient, _ := fakeFactory.Create()

	_, err := fakeClient.CoreV1().Endpoints("default").Create(context.TODO(), &existingEp, v1.CreateOptions{})
	assert.NoError(t, err)

	config := &v1beta1.ClusterConfig{
		Spec: &v1beta1.ClusterSpec{
			API: &v1beta1.APISpec{
				Address:         "1.2.3.4",
				ExternalAddress: "get.k0s.sh",
			},
		},
	}
	r := NewEndpointReconciler(config, &fakeAlwaysLeaderElector{}, fakeFactory)

	assert.NoError(t, r.Init())

	assert.NoError(t, r.reconcileEndpoints())
	e := verifyEndpointAddresses(t, expectedAddresses, fakeFactory)
	assert.Equal(t, "bar", e.ObjectMeta.Annotations["foo"])
}

func verifyEndpointAddresses(t *testing.T, expectedAddresses []string, fakeFactory *fakeClientFactory) *corev1.Endpoints {

	fakeClient, _ := fakeFactory.Create()
	ep, err := fakeClient.CoreV1().Endpoints("default").Get(context.TODO(), "kubernetes", v1.GetOptions{})
	assert.NoError(t, err)
	assert.Equal(t, expectedAddresses, endpointAddressesToStrings(ep.Subsets[0].Addresses))

	return ep
}
