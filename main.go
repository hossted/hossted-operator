/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/marketplacemetering"
	"github.com/aws/aws-sdk-go-v2/service/marketplacemetering/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/google/uuid"
	hosstedcomv1 "github.com/hossted/hossted-operator/api/v1"
	"github.com/hossted/hossted-operator/controllers"
	//+kubebuilder:scaffold:imports
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(hosstedcomv1.AddToScheme(scheme))

	utilruntime.Must(hosstedcomv1.TrivyAddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
	if os.Getenv("CLOUD_PROVIDER") == "AWS" && os.Getenv("MARKET_PLACE") == "enabled" {
		// make sure hossted operator sa has access
		// eksctl create iamserviceaccount --name hossted-operator-controller-manager \
		// --cluster ${CLUSTER_NAME} \
		// --attach-policy-arn arn:aws:iam::aws:policy/AWSMarketplaceMeteringFullAccess \
		// --approve \
		// --override-existing-serviceaccounts
		initRegisterUsage()
	}
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "8f415fe4.hossted.com",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.HosstedProjectReconciler{
		Client:            mgr.GetClient(),
		Scheme:            mgr.GetScheme(),
		ReconcileDuration: lookupReconcileDuration(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Hosstedproject")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}

	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}
	// Env variable check  for HOSSTED_API_URL and HOSSTED_AUTH_TOKEN
	// if os.Getenv("HOSSTED_API_URL") == "" || os.Getenv("HOSSTED_AUTH_TOKEN") == "" || os.Getenv("EMAIL_ID") == "" {
	// 	setupLog.Error(fmt.Errorf("Error: Not able to find HOSSTED_API_URL, EMAIL HOSSTED_AUTH_TOKEN environment"), " variables is not set.")
	// 	os.Exit(1)
	// }

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func lookupReconcileDuration() time.Duration {
	val, exists := os.LookupEnv("RECONCILE_DURATION")
	if !exists {
		return time.Second * 10
	} else {
		v, err := time.ParseDuration(val)
		if err != nil {
			fmt.Println(err)
			// Exit Program if not valid
			os.Exit(1)
		}
		return v
	}
}

func initRegisterUsage() {
	// Load the AWS SDK configuration
	cfg, err := config.LoadDefaultConfig(context.TODO())
	if err != nil {
		log.Fatalf("Unable to load SDK config: %v", err)
	}

	// Create the AWS Marketplace Metering client
	meteringClient := marketplacemetering.NewFromConfig(cfg)

	// Generate a unique nonce for the call
	nonce := uuid.New().String()

	// Call the RegisterUsage API
	result, err := callRegisterUsage(meteringClient, os.Getenv("PRODUCT_CODE"), nonce, 1)
	if err != nil {
		log.Fatalf("Error calling RegisterUsage: %v", err)
	}

	// Process the result
	log.Printf("RegisterUsage successful. Entitlement check result: %v", result)
}

func callRegisterUsage(client *marketplacemetering.Client, productCode, nonce string, publicKeyVersion int32) (*marketplacemetering.RegisterUsageOutput, error) {
	// Create the request input
	input := &marketplacemetering.RegisterUsageInput{
		ProductCode:      aws.String(productCode),
		Nonce:            aws.String(nonce),
		PublicKeyVersion: aws.Int32(publicKeyVersion),
	}

	// Call the RegisterUsage API
	result, err := client.RegisterUsage(context.TODO(), input)
	if err != nil {
		switch err.(type) {
		case *types.CustomerNotEntitledException:
			log.Fatalf("Customer is not entitled to use the product: %v", err)
		case *types.InvalidProductCodeException:
			log.Fatalf("The product code is invalid: %v", err)
		case *types.InvalidRegionException:
			log.Fatalf("RegisterUsage must be called from the same region where the container is running: %v", err)
		case *types.ThrottlingException:
			log.Printf("Throttling detected. Please retry the request later: %v", err)
		default:
			log.Fatalf("Error calling RegisterUsage: %v", err)
		}
	}

	// Return the result from AWS Marketplace
	return result, nil
}
