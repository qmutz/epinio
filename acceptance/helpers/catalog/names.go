// Package catalog contains objects and resources, which are used by multiple tests
package catalog

import (
	"fmt"
	"math/rand"
	"strconv"
	"time"
)

// RandInt return a random integer produced with a new seed every time.
// This guarantees that future test runs won't collide with any possible left overs
// from previous runs.
// More here: https://gobyexample.com/random-numbers
func RandInt() int {
	return rand.New(rand.NewSource(time.Now().UnixNano())).Int() // nolint:gosec // Non-crypto use
}

func NewTmpName(base string) string {
	return base + strconv.Itoa(int(time.Now().Nanosecond()))
}

func NewNamespaceName() string {
	return "namespace-" + strconv.Itoa(int(time.Now().Nanosecond()))
}

func NewAppName() string {
	return "apps-" + strconv.Itoa(int(time.Now().Nanosecond()))
}

func NewCatalogServiceName() string {
	return "service-" + strconv.Itoa(int(time.Now().Nanosecond()))
}

func NewServiceName() string {
	return "service-instance-" + strconv.Itoa(int(time.Now().Nanosecond()))
}

func NewCatalogServiceNamePrefixed(prefix string) string {
	return prefix + "-service-" + strconv.Itoa(int(time.Now().Nanosecond()))
}

func NewServiceNamePrefixed(prefix string) string {
	return prefix + "-service-instance-" + strconv.Itoa(int(time.Now().Nanosecond()))
}

func NewUserCredentials() (string, string) {
	username := strconv.Itoa(RandInt()) + "@epinio.io"
	return username, strconv.Itoa(RandInt())
}

func NewConfigurationName() string {
	return "configuration-" + strconv.Itoa(int(time.Now().Nanosecond())) + strconv.Itoa(RandInt())
}

func GetConfigurationBindingName(namespaceName, configurationName, appName string) string {
	return fmt.Sprintf("svc.namespace-%s.svc-%s.app-%s", namespaceName, configurationName, appName)
}
