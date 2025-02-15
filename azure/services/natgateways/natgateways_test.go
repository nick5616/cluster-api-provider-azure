/*
Copyright 2020 The Kubernetes Authors.

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

package natgateways

import (
	"context"
	"net/http"
	"testing"

	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2019-06-01/network"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/kubernetes/scheme"
	infrav1 "sigs.k8s.io/cluster-api-provider-azure/api/v1beta1"
	"sigs.k8s.io/cluster-api-provider-azure/azure"
	"sigs.k8s.io/cluster-api-provider-azure/azure/services/natgateways/mock_natgateways"
	gomockinternal "sigs.k8s.io/cluster-api-provider-azure/internal/test/matchers/gomock"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1alpha3"
)

func init() {
	_ = clusterv1.AddToScheme(scheme.Scheme)
}

func TestReconcileNatGateways(t *testing.T) {
	testcases := []struct {
		name          string
		tags          infrav1.Tags
		expectedError string
		expect        func(s *mock_natgateways.MockNatGatewayScopeMockRecorder, m *mock_natgateways.MockclientMockRecorder)
	}{
		{
			name: "nat gateways in custom vnet mode",
			tags: infrav1.Tags{
				"Name": "my-vnet",
				"sigs.k8s.io_cluster-api-provider-azure_cluster_test-cluster": "shared",
				"sigs.k8s.io_cluster-api-provider-azure_role":                 "common",
			},
			expectedError: "",
			expect: func(s *mock_natgateways.MockNatGatewayScopeMockRecorder, m *mock_natgateways.MockclientMockRecorder) {
				s.Vnet().Return(&infrav1.VnetSpec{
					ID:   "1234",
					Name: "my-vnet",
				})
				s.ClusterName()
			},
		},
		{
			name: "nat gateway create successfully",
			tags: infrav1.Tags{
				"Name": "my-vnet",
				"sigs.k8s.io_cluster-api-provider-azure_cluster_test-cluster": "owned",
				"sigs.k8s.io_cluster-api-provider-azure_role":                 "common",
			},
			expectedError: "",
			expect: func(s *mock_natgateways.MockNatGatewayScopeMockRecorder, m *mock_natgateways.MockclientMockRecorder) {
				s.Vnet().Return(&infrav1.VnetSpec{
					Name: "my-vnet",
				})
				s.ClusterName()
				s.NatGatewaySpecs().Return([]azure.NatGatewaySpec{
					{
						Name: "my-node-natgateway",
						Subnet: infrav1.SubnetSpec{
							Name: "node-subnet",
							Role: infrav1.SubnetNode,
						},
						NatGatewayIP: infrav1.PublicIPSpec{Name: "pip-node-subnet"},
					},
				})

				s.SubscriptionID().AnyTimes().Return("123")
				s.ResourceGroup().AnyTimes().Return("my-rg")
				m.Get(gomockinternal.AContext(), "my-rg", "my-node-natgateway").Return(network.NatGateway{}, autorest.NewErrorWithResponse("", "", &http.Response{StatusCode: 404}, "Not found")).Times(1)
				s.Location().Return("westus")
				s.SetSubnet(infrav1.SubnetSpec{
					Role: infrav1.SubnetNode,
					Name: "node-subnet",
					NatGateway: infrav1.NatGateway{
						ID:   "/subscriptions/123/resourceGroups/my-rg/providers/Microsoft.Network/natGateways/my-node-natgateway",
						Name: "my-node-natgateway",
						NatGatewayIP: infrav1.PublicIPSpec{
							Name: "pip-node-subnet",
						},
					},
				})
				m.CreateOrUpdate(gomockinternal.AContext(), "my-rg", "my-node-natgateway", gomock.AssignableToTypeOf(network.NatGateway{})).Times(1)
			},
		},
		{
			name: "update nat gateway if actual state does not match desired state",
			tags: infrav1.Tags{
				"Name": "my-vnet",
				"sigs.k8s.io_cluster-api-provider-azure_cluster_test-cluster": "owned",
				"sigs.k8s.io_cluster-api-provider-azure_role":                 "common",
			},
			expectedError: "",
			expect: func(s *mock_natgateways.MockNatGatewayScopeMockRecorder, m *mock_natgateways.MockclientMockRecorder) {
				s.Vnet().Return(&infrav1.VnetSpec{
					Name: "my-vnet",
				})
				s.ClusterName()
				s.NatGatewaySpecs().Return([]azure.NatGatewaySpec{
					{
						Name: "my-node-natgateway",
						Subnet: infrav1.SubnetSpec{
							Name: "node-subnet",
							Role: infrav1.SubnetNode,
						},
						NatGatewayIP: infrav1.PublicIPSpec{
							Name: "different-pip-name",
						},
					},
				})

				s.SubscriptionID().AnyTimes().Return("123")
				s.ResourceGroup().Return("my-rg").AnyTimes()
				m.Get(gomockinternal.AContext(), "my-rg", "my-node-natgateway").Times(1).Return(network.NatGateway{
					Name: to.StringPtr("my-node-natgateway"),
					ID:   to.StringPtr("/subscriptions/123/resourceGroups/my-rg/providers/Microsoft.Network/natGateways/my-node-natgateway"),
					NatGatewayPropertiesFormat: &network.NatGatewayPropertiesFormat{PublicIPAddresses: &[]network.SubResource{
						{ID: to.StringPtr("/subscriptions/123/resourceGroups/my-rg/providers/Microsoft.Network/publicIPAddresses/pip-my-node-natgateway-node-subnet-natgw")},
					}},
				}, nil)
				s.SetSubnet(infrav1.SubnetSpec{
					Role: infrav1.SubnetNode,
					Name: "node-subnet",
					NatGateway: infrav1.NatGateway{
						ID:   "/subscriptions/123/resourceGroups/my-rg/providers/Microsoft.Network/natGateways/my-node-natgateway",
						Name: "my-node-natgateway",
						NatGatewayIP: infrav1.PublicIPSpec{
							Name: "different-pip-name",
						},
					},
				})
				s.Location().Return("westus")
				m.CreateOrUpdate(gomockinternal.AContext(), "my-rg", "my-node-natgateway", gomock.AssignableToTypeOf(network.NatGateway{}))
			},
		},
		{
			name: "nat gateway is not updated if it's up to date",
			tags: infrav1.Tags{
				"Name": "my-vnet",
				"sigs.k8s.io_cluster-api-provider-azure_cluster_test-cluster": "owned",
				"sigs.k8s.io_cluster-api-provider-azure_role":                 "common",
			},
			expectedError: "",
			expect: func(s *mock_natgateways.MockNatGatewayScopeMockRecorder, m *mock_natgateways.MockclientMockRecorder) {
				s.Vnet().Return(&infrav1.VnetSpec{
					Name: "my-vnet",
				})
				s.ClusterName()
				s.NatGatewaySpecs().Return([]azure.NatGatewaySpec{
					{
						Name: "my-node-natgateway",
						Subnet: infrav1.SubnetSpec{
							Name: "node-subnet",
							Role: infrav1.SubnetNode,
						},
						NatGatewayIP: infrav1.PublicIPSpec{
							Name: "pip-my-node-natgateway-node-subnet-natgw",
						},
					},
				})

				s.SubscriptionID().AnyTimes().Return("123")
				s.ResourceGroup().Return("my-rg").AnyTimes()
				m.Get(gomockinternal.AContext(), "my-rg", "my-node-natgateway").Times(1).Return(network.NatGateway{
					Name: to.StringPtr("my-node-natgateway"),
					ID:   to.StringPtr("/subscriptions/123/resourceGroups/my-rg/providers/Microsoft.Network/natGateways/my-node-natgateway"),
					NatGatewayPropertiesFormat: &network.NatGatewayPropertiesFormat{PublicIPAddresses: &[]network.SubResource{
						{
							ID: to.StringPtr("/subscriptions/123/resourceGroups/my-rg/providers/Microsoft.Network/publicIPAddresses/pip-my-node-natgateway-node-subnet-natgw"),
						},
					}},
				}, nil)
				s.SetSubnet(infrav1.SubnetSpec{
					Role: infrav1.SubnetNode,
					Name: "node-subnet",
					NatGateway: infrav1.NatGateway{
						ID:   "/subscriptions/123/resourceGroups/my-rg/providers/Microsoft.Network/natGateways/my-node-natgateway",
						Name: "my-node-natgateway",
						NatGatewayIP: infrav1.PublicIPSpec{
							Name: "pip-my-node-natgateway-node-subnet-natgw",
						},
					},
				})
				s.Location().Return("westus").Times(0)
				m.CreateOrUpdate(gomockinternal.AContext(), "my-rg", "my-node-natgateway", gomock.AssignableToTypeOf(network.NatGateway{})).Times(0)
			},
		},
		{
			name: "fail when getting existing nat gateway",
			tags: infrav1.Tags{
				"Name": "my-vnet",
				"sigs.k8s.io_cluster-api-provider-azure_cluster_test-cluster": "owned",
				"sigs.k8s.io_cluster-api-provider-azure_role":                 "common",
			},
			expectedError: "failed to get nat gateway my-node-natgateway in my-rg: #: Internal Server Error: StatusCode=500",
			expect: func(s *mock_natgateways.MockNatGatewayScopeMockRecorder, m *mock_natgateways.MockclientMockRecorder) {
				s.Vnet().Return(&infrav1.VnetSpec{
					Name: "my-vnet",
				})
				s.ClusterName()
				s.NatGatewaySpecs().Return([]azure.NatGatewaySpec{
					{
						Name: "my-node-natgateway",
						Subnet: infrav1.SubnetSpec{
							Name: "node-subnet",
							Role: infrav1.SubnetNode,
						},
					},
				})
				s.ResourceGroup().AnyTimes().Return("my-rg")
				m.Get(gomockinternal.AContext(), "my-rg", "my-node-natgateway").Return(network.NatGateway{}, autorest.NewErrorWithResponse("", "", &http.Response{StatusCode: 500}, "Internal Server Error"))
				m.CreateOrUpdate(gomockinternal.AContext(), gomock.Any(), gomock.Any(), gomock.AssignableToTypeOf(network.NatGateway{})).Times(0)
			},
		},
		{
			name: "fail to create a nat gateway",
			tags: infrav1.Tags{
				"Name": "my-vnet",
				"sigs.k8s.io_cluster-api-provider-azure_cluster_test-cluster": "owned",
				"sigs.k8s.io_cluster-api-provider-azure_role":                 "common",
			},
			expectedError: "failed to create nat gateway my-node-natgateway in resource group my-rg: #: Internal Server Error: StatusCode=500",
			expect: func(s *mock_natgateways.MockNatGatewayScopeMockRecorder, m *mock_natgateways.MockclientMockRecorder) {
				s.Vnet().Return(&infrav1.VnetSpec{
					Name: "my-vnet",
				})
				s.ClusterName()
				s.NatGatewaySpecs().Return([]azure.NatGatewaySpec{
					{
						Name: "my-node-natgateway",
						Subnet: infrav1.SubnetSpec{
							Name: "node-subnet",
							Role: infrav1.SubnetNode,
						},
					},
				})
				s.SubscriptionID().AnyTimes().Return("123")
				s.ResourceGroup().AnyTimes().Return("my-rg")
				m.Get(gomockinternal.AContext(), "my-rg", "my-node-natgateway").Return(network.NatGateway{}, autorest.NewErrorWithResponse("", "", &http.Response{StatusCode: 404}, "Not found"))
				s.Location().Return("westus")
				m.CreateOrUpdate(gomockinternal.AContext(), "my-rg", "my-node-natgateway", gomock.AssignableToTypeOf(network.NatGateway{})).Return(autorest.NewErrorWithResponse("", "", &http.Response{StatusCode: 500}, "Internal Server Error"))
			},
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			t.Parallel()
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			scopeMock := mock_natgateways.NewMockNatGatewayScope(mockCtrl)
			clientMock := mock_natgateways.NewMockclient(mockCtrl)

			tc.expect(scopeMock.EXPECT(), clientMock.EXPECT())

			s := &Service{
				Scope:  scopeMock,
				client: clientMock,
			}

			err := s.Reconcile(context.TODO())
			if tc.expectedError != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(MatchError(tc.expectedError))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func TestDeleteNatGateway(t *testing.T) {
	testcases := []struct {
		name          string
		tags          infrav1.Tags
		expectedError string
		expect        func(s *mock_natgateways.MockNatGatewayScopeMockRecorder, m *mock_natgateways.MockclientMockRecorder)
	}{
		{
			name: "nat gateways in custom vnet mode",
			tags: infrav1.Tags{
				"Name": "my-vnet",
				"sigs.k8s.io_cluster-api-provider-azure_cluster_test-cluster": "shared",
				"sigs.k8s.io_cluster-api-provider-azure_role":                 "common",
			},
			expectedError: "",
			expect: func(s *mock_natgateways.MockNatGatewayScopeMockRecorder, m *mock_natgateways.MockclientMockRecorder) {
				s.Vnet().Return(&infrav1.VnetSpec{
					ID:   "1234",
					Name: "my-vnet",
				})
				s.ClusterName()
			},
		},
		{
			name: "nat gateway deleted successfully",
			tags: infrav1.Tags{
				"Name": "my-vnet",
				"sigs.k8s.io_cluster-api-provider-azure_cluster_test-cluster": "owned",
				"sigs.k8s.io_cluster-api-provider-azure_role":                 "common",
			},
			expectedError: "",
			expect: func(s *mock_natgateways.MockNatGatewayScopeMockRecorder, m *mock_natgateways.MockclientMockRecorder) {
				s.Vnet().Return(&infrav1.VnetSpec{
					Name: "my-vnet",
				})
				s.ClusterName()
				s.NatGatewaySpecs().Return([]azure.NatGatewaySpec{
					{
						Name: "my-node-natgateway",
						Subnet: infrav1.SubnetSpec{
							Name: "node-subnet",
							Role: infrav1.SubnetNode,
						},
					},
				})
				s.ResourceGroup().Return("my-rg")
				m.Delete(gomockinternal.AContext(), "my-rg", "my-node-natgateway")
			},
		},
		{
			name: "nat gateway already deleted",
			tags: infrav1.Tags{
				"Name": "my-vnet",
				"sigs.k8s.io_cluster-api-provider-azure_cluster_test-cluster": "owned",
				"sigs.k8s.io_cluster-api-provider-azure_role":                 "common",
			},
			expectedError: "",
			expect: func(s *mock_natgateways.MockNatGatewayScopeMockRecorder, m *mock_natgateways.MockclientMockRecorder) {
				s.Vnet().Return(&infrav1.VnetSpec{
					Name: "my-vnet",
				})
				s.ClusterName()
				s.NatGatewaySpecs().Return([]azure.NatGatewaySpec{
					{
						Name: "my-node-natgateway",
						Subnet: infrav1.SubnetSpec{
							Name: "node-subnet",
							Role: infrav1.SubnetNode,
						},
					},
				})
				s.ResourceGroup().Return("my-rg")
				m.Delete(gomockinternal.AContext(), "my-rg", "my-node-natgateway").Return(autorest.NewErrorWithResponse("", "", &http.Response{
					StatusCode: 404,
				}, "Not Found"))
			},
		},
		{
			name: "nat gateway deletion fails",
			tags: infrav1.Tags{
				"Name": "my-vnet",
				"sigs.k8s.io_cluster-api-provider-azure_cluster_test-cluster": "owned",
				"sigs.k8s.io_cluster-api-provider-azure_role":                 "common",
			},
			expectedError: "failed to delete nat gateway my-node-natgateway in resource group my-rg: #: Internal Server Error: StatusCode=500",
			expect: func(s *mock_natgateways.MockNatGatewayScopeMockRecorder, m *mock_natgateways.MockclientMockRecorder) {
				s.Vnet().Return(&infrav1.VnetSpec{
					Name: "my-vnet",
				})
				s.ClusterName()
				s.NatGatewaySpecs().Return([]azure.NatGatewaySpec{
					{
						Name: "my-node-natgateway",
						Subnet: infrav1.SubnetSpec{
							Name: "node-subnet",
							Role: infrav1.SubnetNode,
						},
					},
				})
				s.ResourceGroup().AnyTimes().Return("my-rg")
				m.Delete(gomockinternal.AContext(), "my-rg", "my-node-natgateway").Return(autorest.NewErrorWithResponse("", "", &http.Response{
					StatusCode: 500,
				}, "Internal Server Error"))
			},
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			g := NewWithT(t)
			t.Parallel()
			mockCtrl := gomock.NewController(t)
			defer mockCtrl.Finish()
			scopeMock := mock_natgateways.NewMockNatGatewayScope(mockCtrl)
			clientMock := mock_natgateways.NewMockclient(mockCtrl)

			tc.expect(scopeMock.EXPECT(), clientMock.EXPECT())

			s := &Service{
				Scope:  scopeMock,
				client: clientMock,
			}

			err := s.Delete(context.TODO())
			if tc.expectedError != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(MatchError(tc.expectedError))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}
