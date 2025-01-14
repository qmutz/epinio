package usercmd

import (
	"runtime"

	"github.com/epinio/epinio/internal/selfupdater"
	"github.com/epinio/epinio/internal/version"
	"github.com/pkg/errors"
)

// ClientSync downloads the epinio client binary matching the current OS and
// architecture and replaces the currently running one.
func (c *EpinioClient) ClientSync() error {
	log := c.Log.WithName("Client sync")
	log.Info("start")
	defer log.Info("return")

	v, err := c.API.Info()
	if err != nil {
		return err
	}

	if version.Version == v.Version {
		c.ui.Success().Msgf("Client and server version are the same (%s). Nothing to do!", v.Version)

		return nil
	}

	updater, err := getUpdater()
	if err != nil {
		return errors.Wrap(err, "getting an updater")
	}

	err = updater.Update(v.Version)
	if err != nil {
		return errors.Wrap(err, "updating the client")
	}

	c.ui.Success().Msgf("Updated epinio client to %s", v.Version)

	return nil
}

func getUpdater() (selfupdater.Updater, error) {
	var updater selfupdater.Updater
	switch os := runtime.GOOS; os {
	case "linux", "darwin":
		updater = selfupdater.PosixUpdater{}
	case "windows":
		updater = selfupdater.WindowsUpdater{}
	default:
		return nil, errors.New("unknown operating system")
	}

	return updater, nil
}
