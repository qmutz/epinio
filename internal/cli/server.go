package cli

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/epinio/epinio/helpers/termui"
	"github.com/epinio/epinio/helpers/tracelog"
	"github.com/epinio/epinio/internal/cli/server"
	"github.com/epinio/epinio/internal/version"
	"github.com/gin-gonic/gin"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

func init() {
	flags := CmdServer.Flags()

	flags.StringP("namespace", "n", "epinio", "(NAMESPACE) The namespace to use")
	err := viper.BindPFlag("namespace", flags.Lookup("namespace"))
	checkErr(err)
	err = viper.BindEnv("namespace", "NAMESPACE")
	checkErr(err)

	flags.Int("port", 0, "(PORT) The port to listen on. Leave empty to auto-assign a random port")
	err = viper.BindPFlag("port", flags.Lookup("port"))
	checkErr(err)
	err = viper.BindEnv("port", "PORT")
	checkErr(err)

	flags.String("tls-issuer", "", "(TLS_ISSUER) The cluster issuer to use for workload certificates")
	err = viper.BindPFlag("tls-issuer", flags.Lookup("tls-issuer"))
	checkErr(err)
	err = viper.BindEnv("tls-issuer", "TLS_ISSUER")
	checkErr(err)

	flags.String("access-control-allow-origin", "", "(ACCESS_CONTROL_ALLOW_ORIGIN) Domains allowed to use the API")
	err = viper.BindPFlag("access-control-allow-origin", flags.Lookup("access-control-allow-origin"))
	checkErr(err)
	err = viper.BindEnv("access-control-allow-origin", "ACCESS_CONTROL_ALLOW_ORIGIN")
	checkErr(err)

	flags.String("registry-certificate-secret", "", "(REGISTRY_CERTIFICATE_SECRET) Secret for the registry's TLS certificate")
	err = viper.BindPFlag("registry-certificate-secret", flags.Lookup("registry-certificate-secret"))
	checkErr(err)
	err = viper.BindEnv("registry-certificate-secret", "REGISTRY_CERTIFICATE_SECRET")
	checkErr(err)

	flags.String("s3-certificate-secret", "", "(S3_CERTIFICATE_SECRET) Secret for the S3 endpoint TLS certificate. Can be left empty if S3 is served with a trusted certificate.")
	err = viper.BindPFlag("s3-certificate-secret", flags.Lookup("s3-certificate-secret"))
	checkErr(err)
	err = viper.BindEnv("s3-certificate-secret", "S3_CERTIFICATE_SECRET")
	checkErr(err)

	flags.String("trace-output", "text", "(TRACE_OUTPUT) logs output format [text,json]")
	err = viper.BindPFlag("trace-output", flags.Lookup("trace-output"))
	checkErr(err)
	err = viper.BindEnv("trace-output", "TRACE_OUTPUT")
	checkErr(err)

	flags.String("ingress-class-name", "", "(INGRESS_CLASS_NAME) Name of the ingress class to use for apps. Leave empty to add no ingressClassName to the ingress.")
	err = viper.BindPFlag("ingress-class-name", flags.Lookup("ingress-class-name"))
	checkErr(err)
	err = viper.BindEnv("ingress-class-name", "INGRESS_CLASS_NAME")
	checkErr(err)

	flags.String("app-image-exporter", "", "(APP_IMAGE_EXPORTER) Name of the container image used to download the application image from the 'export' API.")
	err = viper.BindPFlag("app-image-exporter", flags.Lookup("app-image-exporter"))
	checkErr(err)
	err = viper.BindEnv("app-image-exporter", "APP_IMAGE_EXPORTER")
	checkErr(err)
}

// CmdServer implements the command: epinio server
var CmdServer = &cobra.Command{
	Use:   "server",
	Short: "Starts the Epinio server.",
	Long:  "This command starts the Epinio server. `epinio install` ensures the server is running inside your cluster. Normally you don't need to run this command manually.",
	RunE: func(cmd *cobra.Command, args []string) error {
		cmd.SilenceUsage = true
		logger := tracelog.NewLogger().WithName("EpinioServer")

		handler, err := server.NewHandler(logger)
		if err != nil {
			return errors.Wrap(err, "error creating handler")
		}

		port := viper.GetInt("port")
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			return errors.Wrap(err, "error creating listener")
		}

		ui := termui.NewUI()
		ui.Normal().Msg("Epinio version: " + version.Version)
		listeningPort := strconv.Itoa(listener.Addr().(*net.TCPAddr).Port)
		ui.Normal().Msg("listening on localhost on port " + listeningPort)

		return startServerGracefully(listener, handler)
	},
}

// startServerGracefully will start the server and will wait for a graceful shutdown
func startServerGracefully(listener net.Listener, handler http.Handler) error {
	srv := &http.Server{
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second, // Prevent Slowloris attack
	}

	quit := make(chan os.Signal, 1)

	// in coverage mode we need to be able to terminate the server to collect the report
	if _, ok := os.LookupEnv("EPINIO_COVERAGE"); ok {
		router := handler.(*gin.Engine)
		router.GET("/exit", func(c *gin.Context) {
			c.AbortWithStatus(http.StatusNoContent)
			quit <- syscall.SIGTERM
		})
	}

	go func() {
		if err := srv.Serve(listener); err != nil && errors.Is(err, http.ErrServerClosed) {
			log.Printf("listen: %s\n", err)
		}
	}()

	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatal("Server forced to shutdown:", err)
		return err
	}

	log.Println("Server exiting")
	return nil
}
