/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	certclient "github.com/cert-manager/cert-manager/pkg/client/clientset/versioned/typed/certmanager/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"github.com/spf13/cobra"

	"context"
)

// listCmd represents the list command
var listCmd = &cobra.Command{
	Use:   "list",
	Short: "A brief description of your command",
	Run: func(cmd *cobra.Command, args []string) {
		list()
	},
}

var (
	namespace  string
	all        bool
	sortName   bool
	sortReady  bool
	sortFrom   bool
	sortTo     bool
	sortIssuer bool
)

func init() {
	rootCmd.AddCommand(listCmd)

	listCmd.Flags().StringVarP(&namespace, "namespace", "n", "", "namespace")
	listCmd.Flags().BoolVarP(&all, "all", "A", false, "list across all namespaces")
	listCmd.MarkFlagsMutuallyExclusive("namespace", "all")

	listCmd.Flags().BoolVarP(&sortName, "name", "", false, "sort by name")
	listCmd.Flags().BoolVarP(&sortReady, "ready", "", false, "sort by ready-state")
	listCmd.Flags().BoolVarP(&sortFrom, "from", "", false, "sort by from")
	listCmd.Flags().BoolVarP(&sortTo, "to", "", false, "sort by to")
	listCmd.Flags().BoolVarP(&sortIssuer, "issuer", "", false, "sort by issuer")
	listCmd.MarkFlagsMutuallyExclusive("name", "ready", "from", "to", "issuer")
}

func list() {
	certClient, err := getCertmanagerClient()
	if err != nil {
		panic(err)
	}

	ns := getCurrentNamespace()
	if namespace != "" {
		ns = namespace
	}
	if all {
		ns = ""
	}

	if !(sortName || sortReady || sortFrom || sortTo || sortIssuer) {
		sortName = true
	}

	certList, err := certClient.Certificates(ns).List(context.Background(), v1.ListOptions{})
	certs := certList.Items
	sortCerts(certs, sortName, sortReady, sortIssuer, sortFrom, sortTo)
	printCerts(certs)
}

func printCerts(certs []certv1.Certificate) {
	w := tabwriter.NewWriter(os.Stdout, 5, 4, 3, ' ', 0)
	_, _ = fmt.Fprintf(w, "NAMESPACE\tNAME\tREADY\tVALID FROM\tVALID TO\tISSUER\t\n")
	for _, cert := range certs {
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

func sortCerts(certList []certv1.Certificate, sortName, sortReady, sortIssuer, sortFrom, sortTo bool) {
	var sortFunc func(a certv1.Certificate, b certv1.Certificate) int

	switch {
	case sortName:
		sortFunc = func(a certv1.Certificate, b certv1.Certificate) int {
			return strings.Compare(a.Name, b.Name)
		}
	case sortReady:
		sortFunc = func(a certv1.Certificate, b certv1.Certificate) int {
			return strings.Compare(status(a), status(b))
		}
	case sortFrom:
		sortFunc = func(a certv1.Certificate, b certv1.Certificate) int {
			if a.Status.NotBefore == nil {
				return -1
			} else if b.Status.NotBefore == nil {
				return 1
			} else if a.Status.NotBefore.Before(b.Status.NotBefore) {
				return -1
			} else {
				return 1
			}
		}
	case sortTo:
		sortFunc = func(a certv1.Certificate, b certv1.Certificate) int {
			if a.Status.NotAfter == nil {
				return -1
			} else if b.Status.NotAfter == nil {
				return 1
			} else if a.Status.NotAfter.Before(b.Status.NotAfter) {
				return -1
			} else {
				return 1
			}
		}
	case sortIssuer:
		sortFunc = func(a certv1.Certificate, b certv1.Certificate) int {
			return strings.Compare(a.Spec.IssuerRef.Name, b.Spec.IssuerRef.Name)
		}
	}

	if sortFunc == nil {
		panic("sort func not set")
		return
	}
	slices.SortFunc(certList, sortFunc)
}

func status(cert certv1.Certificate) string {
	status := ""
	for _, cond := range cert.Status.Conditions {
		if cond.Type == certv1.CertificateConditionReady {
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

func getCertmanagerClient() (*certclient.CertmanagerV1Client, error) {

	config, err := getConfig()
	if err != nil {
		return nil, err
	}

	// create the cert manager clientset
	return certclient.NewForConfig(config)
}
