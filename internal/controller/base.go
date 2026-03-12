package controller

import (
	"time"

	"github.com/ifo-operator/inflightoperations/lib/logging"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type BaseReconciler struct {
	client.Client
	Log logging.LevelLogger
}

// Reconcile started.
func (r *BaseReconciler) Started() {
	r.Log.Info("Reconcile started.")
}

// Reconcile ended.
func (r *BaseReconciler) Ended(reQin time.Duration, err error) (reQ time.Duration) {
	defer func() {
		r.Log.Info(
			"Reconcile ended.",
			"reQ",
			reQ)
	}()
	reQ = reQin
	if err == nil {
		return
	}
	reQ = Settings.RequeueInterval
	r.Log.Error(
		err,
		"Reconcile failed.")
	return
}
