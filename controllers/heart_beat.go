package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/go-logr/logr"
	hosstedcomv1 "github.com/hossted/hossted-operator/api/v1"
	internalHTTP "github.com/hossted/hossted-operator/pkg/http"
)

func (r *HosstedProjectReconciler) StartPeriodicUpdate(ctx context.Context, instance *hosstedcomv1.Hosstedproject, logger logr.Logger) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if err := r.runClusterUpdate(ctx, instance, logger); err != nil {
				log.Println("Error during periodic update:", err)
			}
		case <-ctx.Done():
			log.Println("Ticker stopped:", ctx.Err())
			return
		}
	}
}

// Extracted function for cluster update logic
func (r *HosstedProjectReconciler) runClusterUpdate(ctx context.Context, instance *hosstedcomv1.Hosstedproject, logger logr.Logger) error {
	collector, _, _, err := r.collector(ctx, instance)
	if err != nil {
		//return err
	}

	for i := range collector {
		collector[i].AppAPIInfo.ClusterUUID = instance.Status.ClusterUUID
		collector[i].AppAPIInfo.HeartBeat = true
		collectorJson, err := json.Marshal(collector[i])
		if err != nil {
			//return err
		}

		appUUIDRegPath := os.Getenv("HOSSTED_API_URL") + "/apps"
		resp, err := internalHTTP.HttpRequest(collectorJson, appUUIDRegPath)
		if err != nil {
			//return err
		}

		logger.Info(fmt.Sprintf("Sending heartbeat req no [%d] to hossted API", i), "appUUID", collector[i].AppAPIInfo.AppUUID, "resp", resp.ResponseBody, "statuscode", resp.StatusCode)
	}

	log.Println("Periodic cluster update successful")
	return nil
}
