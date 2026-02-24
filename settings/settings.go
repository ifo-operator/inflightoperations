package settings

import (
	"os"
	"strconv"
	"time"

	liberr "github.com/mansam/inflightoperations/lib/error"
)

// Environment variables
const (
	EnvDebounceThreshold   = "DEBOUNCE_THRESHOLD"
	EnvInformerSyncTimeout = "INFORMER_SYNC_TIMEOUT"
	EnvK8SAPITimeout       = "K8S_API_TIMEOUT"
	EnvK8SInformerResync   = "K8S_INFORMER_RESYNC"
)

// Defaults
const (
	DefaultDebounceThreshold   = 30
	DefaultInformerSyncTimeout = 30
	DefaultK8SAPITimeout       = 30
	DefaultK8SInformerResync   = 30
)

var Settings ControllerSettings

func init() {
	err := Settings.Load()
	if err != nil {
		panic(err)
	}
}

type ControllerSettings struct {
	DebounceThreshold   time.Duration
	InformerSyncTimeout time.Duration
	K8SAPITimeout       time.Duration
	K8SInformerResync   time.Duration
}

func (r *ControllerSettings) Load() (err error) {
	r.DebounceThreshold, err = LookupSeconds(EnvDebounceThreshold, DefaultDebounceThreshold)
	if err != nil {
		return
	}
	r.InformerSyncTimeout, err = LookupSeconds(EnvInformerSyncTimeout, DefaultInformerSyncTimeout)
	if err != nil {
		return
	}
	r.K8SAPITimeout, err = LookupSeconds(EnvK8SAPITimeout, DefaultK8SAPITimeout)
	if err != nil {
		return
	}
	r.K8SInformerResync, err = LookupSeconds(EnvK8SInformerResync, DefaultK8SInformerResync)
	if err != nil {
		return
	}
	return
}

func LookupInt(envvar string, def int) (val int, err error) {
	str, found := os.LookupEnv(envvar)
	if !found {
		val = def
		return
	}
	val, err = strconv.Atoi(str)
	if err != nil {
		err = liberr.Wrap(err, "var", envvar)
		return
	}
	return
}

func LookupSeconds(envvar string, def int) (seconds time.Duration, err error) {
	str, found := os.LookupEnv(envvar)
	if !found {
		seconds = time.Duration(def) * time.Second
		return
	}
	val, err := strconv.Atoi(str)
	if err != nil {
		err = liberr.Wrap(err, "var", envvar)
		return
	}
	seconds = time.Duration(val) * time.Second
	return
}
