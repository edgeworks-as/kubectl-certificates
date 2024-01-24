package main

import (
	"context"
	"flag"
	"fmt"
	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certv1 "github.com/cert-manager/cert-manager/pkg/client/clientset/versioned/typed/certmanager/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"os"
	"path/filepath"
	"text/tabwriter"
	"time"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	cmapi.AddToScheme(scheme)
}

func main() {
	certClient, err := getCertmanagerClient()
	if err != nil {
		panic(err)
	}

	certs, err := certClient.Certificates("").List(context.Background(), v1.ListOptions{})

	w := tabwriter.NewWriter(os.Stdout, 5, 4, 3, ' ', 0)
	fmt.Fprintf(w, "NAMESPACE\tNAME\tREADY\tVALID FROM\tVALID TO\tISSUER\t\n")
	for _, cert := range certs.Items {
		status := ""
		for _, cond := range cert.Status.Conditions {
			if cond.Type == cmapi.CertificateConditionReady {
				status = string(cond.Status)
				break
			}
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t\n",
			cert.Namespace,
			cert.Name,
			status,
			cert.Status.NotBefore.Format(time.RFC3339),
			cert.Status.NotAfter.Format(time.RFC3339),
			cert.Spec.IssuerRef.Name)
	}
	w.Flush()
}

func getConfig() (*rest.Config, error) {
	var kubeConfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeConfig = flag.String("kubeConfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeConfig file")
	} else {
		kubeConfig = flag.String("kubeConfig", "", "absolute path to the kubeConfig file")
	}
	flag.Parse()

	// use the current context in kubeConfig
	return clientcmd.BuildConfigFromFlags("", *kubeConfig)
}

func getCoreClient() (*kubernetes.Clientset, error) {

	config, err := getConfig()
	if err != nil {
		return nil, err
	}

	// create the core clientset
	return kubernetes.NewForConfig(config)
}

func getCertmanagerClient() (*certv1.CertmanagerV1Client, error) {

	config, err := getConfig()
	if err != nil {
		return nil, err
	}

	// create the cert manager clientset
	return certv1.NewForConfig(config)
}
