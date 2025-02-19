//go:build unit || !integration

package test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/bacalhau-project/bacalhau/pkg/publicapi/client"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/bacalhau-project/bacalhau/pkg/logger"
	"github.com/bacalhau-project/bacalhau/pkg/model"
	"github.com/bacalhau-project/bacalhau/pkg/node"
	testutils "github.com/bacalhau-project/bacalhau/pkg/test/utils"
)

// Define the suite, and absorb the built-in basic suite
// functionality from testify - including a T() method which
// returns the current testing context
type WebsocketSuite struct {
	suite.Suite
	node   *node.Node
	client *client.APIClient
}

// In order for 'go test' to run this suite, we need to create
// a normal test function and pass our suite to suite.Run
func TestWebsocketSuite(t *testing.T) {
	suite.Run(t, new(WebsocketSuite))
}

// Before each test
func (s *WebsocketSuite) SetupTest() {
	logger.ConfigureTestLogging(s.T())
	n, _ := setupNodeForTest(s.T())
	s.node = n
	s.client = client.NewAPIClient(n.APIServer.Address, n.APIServer.Port)
}

// After each test
func (s *WebsocketSuite) TearDownTest() {
	s.node.CleanupManager.Cleanup(context.Background())
}

func (s *WebsocketSuite) TestWebsocketEverything() {
	ctx := context.Background()
	// string.Replace http with ws in c.BaseURI
	url := *s.client.BaseURI
	url.Scheme = "ws"
	wurl := url.JoinPath("api", "v1", "requester", "websocket", "events")

	conn, _, err := websocket.DefaultDialer.Dial(wurl.String(), nil)
	require.NoError(s.T(), err)
	s.T().Cleanup(func() {
		s.NoError(conn.Close())
	})

	eventChan := make(chan model.JobEvent)
	go func() {
		defer close(eventChan)
		for {
			var event model.JobEvent
			err = conn.ReadJSON(&event)
			if errors.Is(err, net.ErrClosed) {
				return
			}
			require.NoError(s.T(), err)
			eventChan <- event
		}
	}()

	// Pause to ensure the websocket connects _before_ we submit the job
	time.Sleep(100 * time.Millisecond)

	genericJob := testutils.MakeNoopJob(s.T())
	_, err = s.client.Submit(ctx, genericJob)
	require.NoError(s.T(), err)

	event := <-eventChan
	require.Equal(s.T(), model.JobEventCreated, event.EventName)

}

func (s *WebsocketSuite) TestWebsocketSingleJob() {
	s.T().Skip("TODO: test is flaky as by the time we connect to the websocket, " +
		"the job has already progressed and first event is not guaranteed to be 'Created'")
	ctx := context.Background()

	genericJob := testutils.MakeNoopJob(s.T())
	j, err := s.client.Submit(ctx, genericJob)
	require.NoError(s.T(), err)

	url := *s.client.BaseURI
	url.Scheme = "ws"
	wurl := url.JoinPath("websocket", "events")
	wurl.RawQuery = fmt.Sprintf("job_id=%s", j.Metadata.ID)

	conn, _, err := websocket.DefaultDialer.Dial(wurl.String(), nil)
	require.NoError(s.T(), err)

	var event model.JobEvent
	err = conn.ReadJSON(&event)
	require.NoError(s.T(), err)
	require.Equal(s.T(), "Created", event.EventName.String())
}
