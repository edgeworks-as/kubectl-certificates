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

type cert struct {
	C      certv1.Certificate
	Issues []string
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

	clusterIssuerList, err := certClient.ClusterIssuers().List(context.Background(), v1.ListOptions{})
	if err != nil {
		panic(err)
	}

	issuerList, err := certClient.Issuers(ns).List(context.Background(), v1.ListOptions{})
	if err != nil {
		panic(err)
	}

	certList, err := certClient.Certificates(ns).List(context.Background(), v1.ListOptions{})
	if err != nil {
		panic(err)
	}
	certs := convert(certList.Items)

	validate(certs, clusterIssuerList, issuerList)

	sortCerts(certs, sortName, sortReady, sortIssuer, sortFrom, sortTo)
	printCerts(certs)
}

func validate(certs []*cert, clusterIssuersList *certv1.ClusterIssuerList, issuersList *certv1.IssuerList) {

	clusterIssuers := make(map[string]*certv1.ClusterIssuer)
	issuers := make(map[string]*certv1.Issuer)

	for _, iss := range clusterIssuersList.Items {
		clusterIssuers[iss.Name] = &iss
	}

	for _, iss := range issuersList.Items {
		issuers[iss.Name] = &iss
	}

	for _, c := range certs {
		switch c.C.Spec.IssuerRef.Kind {
		case "ClusterIssuer":
			if _, found := clusterIssuers[c.C.Spec.IssuerRef.Name]; !found {
				c.Issues = append(c.Issues, "Invalid cluster issuer.")
			}
		case "Issuer":
			if _, found := issuers[c.C.Spec.IssuerRef.Name]; !found {
				c.Issues = append(c.Issues, "Invalid issuer.")
			}
		}
	}
}

func convert(items []certv1.Certificate) []*cert {
	var result []*cert

	for _, c := range items {
		result = append(result, &cert{C: c})
	}

	return result
}

func printCerts(certs []*cert) {
	w := tabwriter.NewWriter(os.Stdout, 5, 4, 3, ' ', 0)
	_, _ = fmt.Fprintf(w, "NAMESPACE\tNAME\tREADY\tVALID FROM\tVALID TO\tISSUER\tISSUES\n")
	for _, cert := range certs {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t\n",
			cert.C.Namespace,
			cert.C.Name,
			status(cert),
			formatTime(cert.C.Status.NotBefore),
			formatTime(cert.C.Status.NotAfter),
			cert.C.Spec.IssuerRef.Name,
			strings.Join(cert.Issues, " "))
	}
	_ = w.Flush()
}

func sortCerts(certList []*cert, sortName, sortReady, sortIssuer, sortFrom, sortTo bool) {
	var sortFunc func(a *cert, b *cert) int

	switch {
	case sortName:
		sortFunc = func(a *cert, b *cert) int {
			return strings.Compare(a.C.Name, b.C.Name)
		}
	case sortReady:
		sortFunc = func(a *cert, b *cert) int { return strings.Compare(status(a), status(b)) }
	case sortFrom:
		sortFunc = func(a *cert, b *cert) int {
			if a.C.Status.NotBefore == nil {
				return -1
			} else if b.C.Status.NotBefore == nil {
				return 1
			} else if a.C.Status.NotBefore.Before(b.C.Status.NotBefore) {
				return -1
			} else {
				return 1
			}
		}
	case sortTo:
		sortFunc = func(a *cert, b *cert) int {
			if a.C.Status.NotAfter == nil {
				return -1
			} else if b.C.Status.NotAfter == nil {
				return 1
			} else if a.C.Status.NotAfter.Before(b.C.Status.NotAfter) {
				return -1
			} else {
				return 1
			}
		}
	case sortIssuer:
		sortFunc = func(a *cert, b *cert) int {
			return strings.Compare(a.C.Spec.IssuerRef.Name, b.C.Spec.IssuerRef.Name)
		}
	}

	if sortFunc == nil {
		panic("sort func not set")
		return
	}
	slices.SortFunc(certList, sortFunc)
}

func status(cert *cert) string {
	status := ""
	for _, cond := range cert.C.Status.Conditions {
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
