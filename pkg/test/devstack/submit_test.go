//go:build integration || !unit

package devstack

import (
	"context"
	"testing"

	"github.com/bacalhau-project/bacalhau/pkg/publicapi/client"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/bacalhau-project/bacalhau/pkg/devstack"
	"github.com/bacalhau-project/bacalhau/pkg/setup"
	testutils "github.com/bacalhau-project/bacalhau/pkg/test/teststack"

	"github.com/bacalhau-project/bacalhau/pkg/logger"
	_ "github.com/bacalhau-project/bacalhau/pkg/logger"
	"github.com/bacalhau-project/bacalhau/pkg/model"
)

type DevstackSubmitSuite struct {
	suite.Suite
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestDevstackSubmitSuite(t *testing.T) {
	suite.Run(t, new(DevstackSubmitSuite))
}

// Before each test
func (suite *DevstackSubmitSuite) SetupTest() {
	logger.ConfigureTestLogging(suite.T())
	setup.SetupBacalhauRepoForTesting(suite.T())
}

func (suite *DevstackSubmitSuite) TestEmptySpec() {
	ctx := context.Background()

	stack := testutils.Setup(ctx, suite.T(),
		devstack.WithNumberOfHybridNodes(1),
	)

	apiServer := stack.Nodes[0].APIServer
	apiClient := client.NewAPIClient(apiServer.Address, apiServer.Port)

	j := &model.Job{}
	j.Spec.Deal = model.Deal{Concurrency: 1}
	_, missingSpecError := apiClient.Submit(ctx, j)

	require.Error(suite.T(), missingSpecError)

	j = &model.Job{}
	j.Spec = model.Spec{EngineSpec: model.NewEngineBuilder().WithType(model.EngineDocker.String()).Build()}
	_, missingDealError := apiClient.Submit(ctx, j)
	require.Error(suite.T(), missingDealError)
}
