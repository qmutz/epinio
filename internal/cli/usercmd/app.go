package usercmd

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v2"

	"github.com/epinio/epinio/helpers/bytes"
	"github.com/epinio/epinio/helpers/kubernetes/tailer"
	api "github.com/epinio/epinio/internal/api/v1"
	"github.com/epinio/epinio/internal/cli/logprinter"
	"github.com/epinio/epinio/pkg/api/core/v1/models"

	"github.com/gorilla/websocket"
	"github.com/pkg/errors"
)

// AppCreate creates an app without a workload
func (c *EpinioClient) AppCreate(appName string, appConfig models.ApplicationUpdateRequest) error {
	log := c.Log.WithName("Apps").WithValues("Organization", c.Config.Org, "Application", appName)
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	c.ui.Note().
		WithStringValue("Organization", c.Config.Org).
		WithStringValue("Application", appName).
		Msg("Create application")

	details.Info("create application")

	request := models.ApplicationCreateRequest{
		Name:          appName,
		Configuration: appConfig,
	}

	_, err := c.API.AppCreate(request, c.Config.Org)
	if err != nil {
		return err
	}

	c.ui.Success().Msg("Ok")
	return nil
}

// AppsMatching returns all Epinio apps having the specified prefix
// in their name.
func (c *EpinioClient) AppsMatching(ctx context.Context, prefix string) []string {
	log := c.Log.WithName("AppsMatching").WithValues("PrefixToMatch", prefix)
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	result := []string{}

	// Ask for all apps. Filtering is local.
	// TODO: Create new endpoint (compare `EnvMatch`) and move filtering to the server.

	apps, err := c.API.Apps(c.Config.Org)
	if err != nil {
		return result
	}

	for _, app := range apps {
		details.Info("Found", "Name", app.Meta.Name)

		if strings.HasPrefix(app.Meta.Name, prefix) {
			details.Info("Matched", "Name", app.Meta.Name)
			result = append(result, app.Meta.Name)
		}
	}

	sort.Strings(result)

	return result
}

// Apps gets all Epinio apps in the targeted org, or all apps in all namespaces
func (c *EpinioClient) Apps(all bool) error {
	log := c.Log.WithName("Apps").WithValues("Namespace", c.Config.Org)
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	msg := c.ui.Note()
	if all {
		msg.Msg("Listing all applications")
	} else {
		msg.
			WithStringValue("Namespace", c.Config.Org).
			Msg("Listing applications")

		if err := c.TargetOk(); err != nil {
			return err
		}
	}

	details.Info("list applications")

	var apps models.AppList
	var err error

	if all {
		apps, err = c.API.AllApps()
	} else {
		apps, err = c.API.Apps(c.Config.Org)
	}
	if err != nil {
		return err
	}

	sort.Sort(apps)

	if all {
		msg = c.ui.Success().WithTable("Namespace", "Name", "Status", "Routes", "Services", "Status Details")

		for _, app := range apps {
			if app.Workload == nil {
				msg = msg.WithTableRow(
					app.Meta.Org,
					app.Meta.Name,
					"n/a",
					"n/a",
					strings.Join(app.Configuration.Services, ", "),
					app.StatusMessage,
				)
			} else {
				msg = msg.WithTableRow(
					app.Meta.Org,
					app.Meta.Name,
					app.Workload.Status,
					app.Workload.Route,
					strings.Join(app.Configuration.Services, ", "),
					app.StatusMessage,
				)
			}
		}
	} else {
		msg = c.ui.Success().WithTable("Name", "Status", "Routes", "Services", "Status Details")

		for _, app := range apps {
			if app.Workload == nil {
				msg = msg.WithTableRow(
					app.Meta.Name,
					"n/a",
					"n/a",
					strings.Join(app.Configuration.Services, ", "),
					app.StatusMessage,
				)
			} else {
				msg = msg.WithTableRow(
					app.Meta.Name,
					app.Workload.Status,
					app.Workload.Route,
					strings.Join(app.Configuration.Services, ", "),
					app.StatusMessage,
				)
			}
		}
	}

	msg.Msg("Epinio Applications:")

	return nil
}

// AppShow displays the information of the named app, in the targeted org
func (c *EpinioClient) AppShow(appName string) error {
	log := c.Log.WithName("Apps").WithValues("Namespace", c.Config.Org, "Application", appName)
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	c.ui.Note().
		WithStringValue("Namespace", c.Config.Org).
		WithStringValue("Application", appName).
		Msg("Show application details")

	if err := c.TargetOk(); err != nil {
		return err
	}

	details.Info("show application")

	app, err := c.API.AppShow(c.Config.Org, appName)
	if err != nil {
		return err
	}

	msg := c.ui.Success().WithTable("Key", "Value")

	var createdAt time.Time
	if app.Workload != nil {
		createdAt, err = time.Parse(time.RFC3339, app.Workload.CreatedAt)
		if err != nil {
			return err
		}
		msg = msg.WithTableRow("Status", app.Workload.Status).
			WithTableRow("Username", app.Workload.Username).
			WithTableRow("StageId", app.Workload.StageID).
			WithTableRow("Routes", app.Workload.Route).
			WithTableRow("Age", time.Since(createdAt).Round(time.Second).String()).
			WithTableRow("Restarts", strconv.Itoa(int(app.Workload.Restarts))).
			WithTableRow("milliCPUs", strconv.Itoa(int(app.Workload.MilliCPUs))).
			WithTableRow("Memory", bytes.ByteCountIEC(app.Workload.MemoryBytes))
	} else {
		msg = msg.WithTableRow("Status", "not deployed")
	}

	msg = msg.
		WithTableRow("Desired Instances", fmt.Sprintf("%d", *app.Configuration.Instances)).
		WithTableRow("Bound Services", strings.Join(app.Configuration.Services, ", ")).
		WithTableRow("Environment", "")

	if len(app.Configuration.Environment) > 0 {
		for _, ev := range app.Configuration.Environment.List() {
			msg = msg.WithTableRow("  - "+ev.Name, ev.Value)
		}
	}

	msg.Msg("Details:")

	return nil
}

// AppManifest saves the information of the named app, in the targeted org, into a manifest file
func (c *EpinioClient) AppManifest(appName, manifestPath string) error {
	log := c.Log.WithName("Apps").WithValues("Namespace", c.Config.Org, "Application", appName)
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	c.ui.Note().
		WithStringValue("Namespace", c.Config.Org).
		WithStringValue("Application", appName).
		WithStringValue("Destination", manifestPath).
		Msg("Save application details to manifest")

	if err := c.TargetOk(); err != nil {
		return err
	}

	details.Info("show application")

	app, err := c.API.AppShow(c.Config.Org, appName)
	if err != nil {
		return err
	}

	// TODO -- Not available: Origin information. This is not stored server side.

	m := models.ApplicationManifest{}
	m.Name = appName
	m.Configuration = app.Configuration

	yaml, err := yaml.Marshal(m)
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(manifestPath, yaml, 0600)
	if err != nil {
		return err
	}

	c.ui.Success().Msg("Saved")

	return nil
}

// AppStageID returns the stage id of the named app, in the targeted org
func (c *EpinioClient) AppStageID(appName string) (string, error) {
	log := c.Log.WithName("Apps").WithValues("Namespace", c.Config.Org, "Application", appName)
	log.Info("start")
	defer log.Info("return")

	app, err := c.API.AppShow(c.Config.Org, appName)
	if err != nil {
		return "", err
	}

	if app.Workload == nil {
		return "", errors.New("Application has no workload")
	}

	return app.Workload.StageID, nil
}

// AppUpdate updates the specified running application's attributes (e.g. instances)
func (c *EpinioClient) AppUpdate(appName string, appConfig models.ApplicationUpdateRequest) error {
	log := c.Log.WithName("Apps").WithValues("Namespace", c.Config.Org, "Application", appName)
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	c.ui.Note().
		WithStringValue("Namespace", c.Config.Org).
		WithStringValue("Application", appName).
		Msg("Update application")

	if err := c.TargetOk(); err != nil {
		return err
	}

	details.Info("update application")

	_, err := c.API.AppUpdate(appConfig, c.Config.Org, appName)
	if err != nil {
		return err
	}

	c.ui.Success().Msg("Successfully updated application")

	return nil
}

// AppLogs streams the logs of all the application instances, in the targeted org
// If stageID is an empty string, runtime application logs are streamed. If stageID
// is set, then the matching staging logs are streamed.
// There are 2 ways of stopping this method:
// 1. The websocket connection closes.
// 2. Something is sent to the interrupt channel
// The interrupt channel is used by the caller when printing of logs should
// be stopped.
// To make sure everything is properly stopped (both the main thread and the
// go routine) no matter what caused the stop (number 1 or 2 above):
// - The go routines closes the connection on interrupt. This causes the main
//   loop to stop as well.
// - The main thread sends a signal to the `done` channel when it returns. This
//   causes the go routine to stop.
// - The main thread waits for the go routine to stop before finally returning (by
//   calling `wg.Wait()`.
// This is what happens when `interrupt` receives something:
// 1. The go routine closes the connection
// 2. The loop in the main thread is stopped because the connection was closed
// 3. The main thread sends to the `done` chan (as a "defer" function), and then
//    calls wg.Wait() to wait for the go routine to exit.
// 4. The go routine receives the `done` message, calls wg.Done() and returns
// 5. The main thread returns
// When the connection is closed (e.g. from the server side), the process is the
// same but starts from #2 above.
// TODO move into transport package
func (c *EpinioClient) AppLogs(appName, stageID string, follow bool, interrupt chan bool) error {
	log := c.Log.WithName("Apps").WithValues("Namespace", c.Config.Org, "Application", appName)
	log.Info("start")
	defer log.Info("return")
	details := log.V(1) // NOTE: Increment of level, not absolute.

	c.ui.Note().
		WithStringValue("Namespace", c.Config.Org).
		WithStringValue("Application", appName).
		Msg("Streaming application logs")

	if err := c.TargetOk(); err != nil {
		return err
	}

	details.Info("application logs")

	var urlArgs = []string{}
	urlArgs = append(urlArgs, fmt.Sprintf("follow=%t", follow))
	urlArgs = append(urlArgs, fmt.Sprintf("stage_id=%s", stageID))

	headers := http.Header{
		"Authorization": {"Basic " + base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", c.Config.User, c.Config.Password)))},
	}

	var endpoint string
	if stageID == "" {
		endpoint = api.Routes.Path("AppLogs", c.Config.Org, appName)
	} else {
		endpoint = api.Routes.Path("StagingLogs", c.Config.Org, stageID)
	}
	webSocketConn, resp, err := websocket.DefaultDialer.Dial(
		fmt.Sprintf("%s%s/%s?%s", c.API.WsURL, api.Root, endpoint, strings.Join(urlArgs, "&")), headers)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("Failed to connect to websockets endpoint. Response was = %+v\nThe error is", resp))
	}

	done := make(chan bool)
	// When we get an interrupt, we close the websocket connection and we
	// we don't want to return an error in this case.
	connectionClosedByUs := false

	var wg sync.WaitGroup
	wg.Add(1)
	defer wg.Wait()
	go func() { // Closes the connection on "interrupt" or just stops on "done"
		defer wg.Done()
		for {
			select {
			case <-done: // Used by the other loop stop stop this go routine
				return
			case <-interrupt:
				// Used by the caller of this method to stop everything. We simply close
				// the connection here. This will make the loop below to stop and send us
				// a signal on "done" above. That will stop this go routine too.
				// nolint:errcheck // no place to pass any error to.
				webSocketConn.WriteControl(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""), time.Time{})
				connectionClosedByUs = true
				webSocketConn.Close()
			}
		}
	}()

	defer func() {
		done <- true // Stop the go routine when we return
	}()

	var logLine tailer.ContainerLogLine
	printer := logprinter.LogPrinter{Tmpl: logprinter.DefaultSingleNamespaceTemplate()}
	for {
		_, message, err := webSocketConn.ReadMessage()
		if err != nil {
			if connectionClosedByUs {
				return nil
			}
			if websocket.IsCloseError(err, websocket.CloseNormalClosure) {
				webSocketConn.Close()
				return nil
			}
			return err
		}
		err = json.Unmarshal(message, &logLine)
		if err != nil {
			return err
		}

		printer.Print(logprinter.Log{
			Message:       logLine.Message,
			Namespace:     logLine.Namespace,
			PodName:       logLine.PodName,
			ContainerName: logLine.ContainerName,
		}, c.ui.ProgressNote().Compact())
	}
}

// Delete removes the named application from the cluster
func (c *EpinioClient) Delete(ctx context.Context, appname string) error {
	log := c.Log.WithName("Delete").WithValues("Application", appname)
	log.Info("start")
	defer log.Info("return")

	c.ui.Note().
		WithStringValue("Name", appname).
		WithStringValue("Namespace", c.Config.Org).
		Msg("Deleting application...")

	if err := c.TargetOk(); err != nil {
		return err
	}

	s := c.ui.Progressf("Deleting %s in %s", appname, c.Config.Org)
	defer s.Stop()

	response, err := c.API.AppDelete(c.Config.Org, appname)
	if err != nil {
		return err
	}

	unboundServices := response.UnboundServices
	if len(unboundServices) > 0 {
		s.Stop()

		sort.Strings(unboundServices)
		msg := c.ui.Note().WithTable("Unbound Services")

		for _, bonded := range unboundServices {
			msg = msg.WithTableRow(bonded)
		}
		msg.Msg("")
	}

	c.ui.Success().Msg("Application deleted.")

	return nil
}