/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	v12 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"kubectl-listcerts/internal"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	"github.com/spf13/cobra"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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
	clients, err := internal.NewClient()
	if err != nil {
		fmt.Println("ERROR: %w\n", err)
		return
	}

	ns := clients.CurrentNamespace()
	if namespace != "" {
		ns = namespace
	}
	if all {
		ns = ""
	}

	if !(sortName || sortReady || sortFrom || sortTo || sortIssuer) {
		sortName = true
	}

	clusterIssuerList, err := clients.CertManagerClient().ClusterIssuers().List(context.Background(), v1.ListOptions{})
	if err != nil {
		panic(err)
	}

	issuerList, err := clients.CertManagerClient().Issuers(ns).List(context.Background(), v1.ListOptions{})
	if err != nil {
		panic(err)
	}

	certList, err := clients.CertManagerClient().Certificates(ns).List(context.Background(), v1.ListOptions{})
	if err != nil {
		panic(err)
	}
	certs := convert(certList.Items)

	validate(clients, certs, clusterIssuerList, issuerList)

	sort(certs, sortName, sortReady, sortIssuer, sortFrom, sortTo)
	printCertificatesList(certs)
}

func validate(clients internal.Clients, certs []*cert, clusterIssuersList *certv1.ClusterIssuerList, issuersList *certv1.IssuerList) error {

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
				c.AddIssue("Unknown cluster issuer.")
			}
		case "Issuer":
			if _, found := issuers[c.C.Spec.IssuerRef.Name]; !found {
				c.AddIssue("Unknown issuer.")
			}
		}

		// Check for pending orders
		crs, err := clients.GetCertificateRequestForCertificate(c.C.Name, c.C.Namespace)
		if err != nil {
			return err
		}
		if crs != nil {
			order, err := clients.GetOrderForCertificateRequest(crs.Name, crs.Namespace)
			if err != nil {
				return err
			}
			if order != nil {
				c.AddIssue(fmt.Sprintf("Order status: %s.", order.Status.State))
				auths := []string{"Authorizations:"}
				for _, auth := range order.Status.Authorizations {
					auths = append(auths, fmt.Sprintf("%s: %s", auth.Identifier, auth.InitialState))
				}
				c.AddIssue(fmt.Sprintf("%s.", strings.Join(auths, " ")))
			} else {
				statusTrue := false
				statusMessage := ""
				for _, cond := range crs.Status.Conditions {
					if cond.Type == certv1.CertificateRequestConditionReady {
						statusTrue = cond.Status == v12.ConditionTrue
						statusMessage = cond.Message
						break
					}
				}

				c.AddIssue(fmt.Sprintf("Certificate request status: %t: %s.", statusTrue, statusMessage))
			}
		}

	}
	return nil
}

func convert(items []certv1.Certificate) []*cert {
	var result []*cert

	for _, c := range items {
		result = append(result, &cert{C: c})
	}

	return result
}

func printCertificatesList(certs []*cert) {
	w := tabwriter.NewWriter(os.Stdout, 5, 4, 3, ' ', 0)
	_, _ = fmt.Fprintf(w, "NAMESPACE\tNAME\tREADY\tVALID FROM\tVALID TO\tISSUER\tISSUES\n")
	for _, cert := range certs {
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t\n",
			cert.C.Namespace,
			cert.C.Name,
			cert.Status(),
			formatTime(cert.C.Status.NotBefore),
			formatTime(cert.C.Status.NotAfter),
			cert.C.Spec.IssuerRef.Name,
			strings.Join(cert.Issues, " "))
	}
	_ = w.Flush()
}

func sort(certList []*cert, sortName, sortReady, sortIssuer, sortFrom, sortTo bool) {
	var sortFunc func(a *cert, b *cert) int

	switch {
	case sortName:
		sortFunc = func(a *cert, b *cert) int {
			return strings.Compare(a.C.Name, b.C.Name)
		}
	case sortReady:
		sortFunc = func(a *cert, b *cert) int { return strings.Compare(a.Status(), b.Status()) }
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
	}
	slices.SortFunc(certList, sortFunc)
}

func formatTime(t *v1.Time) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

type cert struct {
	C      certv1.Certificate
	Issues []string
}

func (c *cert) AddIssue(issue string) {
	c.Issues = append(c.Issues, issue)
}

func (c *cert) Status() string {
	status := ""
	for _, cond := range c.C.Status.Conditions {
		if cond.Type == certv1.CertificateConditionReady {
			status = string(cond.Status)
			break
		}
	}
	return status
}
