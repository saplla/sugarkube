package clustersot

import (
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/sugarkube/sugarkube/internal/pkg/provider"
	"github.com/sugarkube/sugarkube/internal/pkg/vars"
	"testing"
)

func TestNewClusterSot(t *testing.T) {
	actual, err := NewClusterSot(KUBECTL)
	assert.Nil(t, err)
	assert.Equal(t, KubeCtlClusterSot{}, actual)
}

type MockClusterSot struct {
	mock.Mock
}

func (m MockClusterSot) IsOnline(sc *vars.StackConfig, values provider.Values) (bool, error) {
	args := m.Called(sc.Cluster)
	return args.Bool(0), args.Error(1)
}

func (m MockClusterSot) IsReady(sc *vars.StackConfig, values provider.Values) (bool, error) {
	args := m.Called(sc.Cluster)
	return args.Bool(0), args.Error(1)
}

func TestIsOnlineTrue(t *testing.T) {
	clusterName := "myCluster"

	// create an instance of our test object
	testObj := MockClusterSot{}

	// setup expectations
	testObj.On("IsOnline", clusterName).Return(true, nil)

	status := vars.ClusterStatus{IsOnline: false}
	sc := vars.StackConfig{
		Cluster: clusterName,
		Status:  status,
	}

	assert.False(t, sc.Status.IsOnline)

	// call the code we are testing
	IsOnline(testObj, &sc, provider.Values{})

	// assert that the expectations were met
	testObj.AssertExpectations(t)

	assert.True(t, sc.Status.IsOnline)
}

func TestIsOnlineFalse(t *testing.T) {
	clusterName := "myCluster"

	// create an instance of our test object
	testObj := MockClusterSot{}

	// setup expectations
	testObj.On("IsOnline", clusterName).Return(false, nil)

	status := vars.ClusterStatus{IsOnline: false}
	sc := vars.StackConfig{
		Cluster: clusterName,
		Status:  status,
	}

	assert.False(t, sc.Status.IsOnline)

	// call the code we are testing
	IsOnline(testObj, &sc, provider.Values{})

	// assert that the expectations were met
	testObj.AssertExpectations(t)

	assert.False(t, sc.Status.IsOnline)
}

func TestIsReadyTrue(t *testing.T) {
	clusterName := "myCluster"

	// create an instance of our test object
	testObj := MockClusterSot{}

	// setup expectations
	testObj.On("IsReady", clusterName).Return(true, nil)

	status := vars.ClusterStatus{IsReady: false}
	sc := vars.StackConfig{
		Cluster: clusterName,
		Status:  status,
	}

	assert.False(t, sc.Status.IsReady)

	// call the code we are testing
	IsReady(testObj, &sc, provider.Values{})

	// assert that the expectations were met
	testObj.AssertExpectations(t)

	assert.True(t, sc.Status.IsReady)
}

func TestIsReadyFalse(t *testing.T) {
	clusterName := "myCluster"

	// create an instance of our test object
	testObj := MockClusterSot{}

	// setup expectations
	testObj.On("IsReady", clusterName).Return(false, nil)

	status := vars.ClusterStatus{IsReady: false}
	sc := vars.StackConfig{
		Cluster: clusterName,
		Status:  status,
	}

	assert.False(t, sc.Status.IsReady)

	// call the code we are testing
	IsReady(testObj, &sc, provider.Values{})

	// assert that the expectations were met
	testObj.AssertExpectations(t)

	assert.False(t, sc.Status.IsReady)
}