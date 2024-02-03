package main

import (
	"context"
	"fmt"
	cmapi "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certv1 "github.com/cert-manager/cert-manager/pkg/client/clientset/versioned/typed/certmanager/v1"
	"github.com/jessevdk/go-flags"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"
)

var (
	scheme = runtime.NewScheme()
)

func init() {
	err := cmapi.AddToScheme(scheme)
	if err != nil {
		panic(err)
	}
}

var opts struct {
	Namespace string `short:"n" long:"namespace" description:"Kubernetes namespace"`
	All       bool   `short:"A" long:"all-namespaces" description:"List for all namespaces"`
	Sort      string `short:"s" long:"sort" description:"Sort by column" choice:"name" choice:"ready" choice:"from" choice:"issuer"`
}

func main() {
	f, err := flags.Parse(&opts)
	if err != nil {
		return
	}

	for _, v := range f {
		fmt.Printf("- %s\n", v)
	}

	certClient, err := getCertmanagerClient()
	if err != nil {
		panic(err)
	}

	ns := getCurrentNamespace()
	if opts.Namespace != "" {
		ns = opts.Namespace
	}
	if opts.All {
		ns = ""
	}

	certs, err := certClient.Certificates(ns).List(context.Background(), v1.ListOptions{})
	certList := certs.Items
	if opts.Sort != "" {
		sortCerts(certList, opts.Sort)
	}

	w := tabwriter.NewWriter(os.Stdout, 5, 4, 3, ' ', 0)
	_, _ = fmt.Fprintf(w, "NAMESPACE\tNAME\tREADY\tVALID FROM\tVALID TO\tISSUER\t\n")
	for _, cert := range certList {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t\n",
			cert.Namespace,
			cert.Name,
			status(cert),
			formatTime(cert.Status.NotBefore),
			formatTime(cert.Status.NotAfter),
			cert.Spec.IssuerRef.Name)
	}
	_ = w.Flush()
}

func sortCerts(certList []cmapi.Certificate, sort string) {
	sortFunc := func(a cmapi.Certificate, b cmapi.Certificate) int {
		switch opts.Sort {
		case "name":
			return strings.Compare(a.Name, b.Name)
		case "ready":
			return strings.Compare(status(a), status(b))
		case "from":
			if a.Status.NotBefore == nil {
				return -1
			} else if b.Status.NotBefore == nil {
				return 1
			} else if a.Status.NotBefore.Before(b.Status.NotBefore) {
				return -1
			} else {
				return 1
			}
		case "issuer":
			return strings.Compare(a.Spec.IssuerRef.Name, b.Spec.IssuerRef.Name)
		}
		return 0
	}
	slices.SortFunc(certList, sortFunc)
}

func status(cert cmapi.Certificate) string {
	status := ""
	for _, cond := range cert.Status.Conditions {
		if cond.Type == cmapi.CertificateConditionReady {
			status = string(cond.Status)
			break
		}
	}
	return status
}
func formatTime(t *v1.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

func getConfig() (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags("", filepath.Join(homedir.HomeDir(), ".kube", "config"))
}

func getCurrentNamespace() string {
	cfg, err := clientcmd.LoadFromFile(filepath.Join(homedir.HomeDir(), ".kube", "config"))
	if err != nil {
		return "default"
	}

	ns := cfg.Contexts[cfg.CurrentContext].Namespace
	if len(ns) == 0 {
		return "default"
	}

	return ns
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
