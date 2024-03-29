/*
Copyright © 2024 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	acmev1 "github.com/cert-manager/cert-manager/pkg/apis/acme/v1"
	metav1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	"io"
	"k8s.io/apimachinery/pkg/util/json"
	"kubectl-listcerts/internal"
	"net"
	"net/http"
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

	certList, err := clients.CertManagerClient().Certificates(ns).List(context.Background(), v1.ListOptions{})
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}
	certs := convert(certList.Items)

	clusterIssuerList, err := clients.CertManagerClient().ClusterIssuers().List(context.Background(), v1.ListOptions{})
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	issuerList, err := clients.CertManagerClient().Issuers(ns).List(context.Background(), v1.ListOptions{})
	if err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

	if err := validate(clients, certs, clusterIssuerList, issuerList); err != nil {
		fmt.Printf("Error: %s\n", err)
		return
	}

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
		issuers[fmt.Sprintf("%s/%s", iss.Namespace, iss.Name)] = &iss
	}

	for i, _ := range certs {
		c := certs[i]

		switch c.C.Spec.IssuerRef.Kind {
		case "ClusterIssuer":
			if _, found := clusterIssuers[c.C.Spec.IssuerRef.Name]; !found {
				c.AddIssue("Wrong cluster issuer.")
			}
		case "Issuer":
			if _, found := issuers[fmt.Sprintf("%s/%s", c.C.Namespace, c.C.Spec.IssuerRef.Name)]; !found {
				c.AddIssue("Wrong issuer.")
			}
		}

		// Check for pending orders
		crs, err := clients.GetCertificateRequestForCertificate(&c.C)
		if err != nil {
			return err
		}
		if crs != nil {
			order, err := clients.GetOrderForCertificateRequest(crs)
			if err != nil {
				return err
			}
			if order != nil {
				if len(order.Status.Certificate) == 0 {
					chall, err := clients.GetChallengesForOrder(order)
					if err != nil {
						return err
					}
					if len(chall) > 0 {
						c.AddIssue(fmt.Sprintf("Order status: %s.", order.Status.State))
						for _, chall := range chall {
							if chall.Status.State == acmev1.Pending {
								if strings.Contains(chall.Status.Reason, "not yet propagated") && chall.Spec.Solver.DNS01 != nil && chall.Spec.Solver.DNS01.AzureDNS != nil {
									subdomain := strings.ReplaceAll(chall.Spec.DNSName, chall.Spec.Solver.DNS01.AzureDNS.HostedZoneName, "")
									domainPaths := strings.Split(subdomain, ".")
									if len(domainPaths) > 1 {
										cname := fmt.Sprintf("_acme-challenge.%s", chall.Spec.DNSName)
										_, err := net.LookupCNAME(cname)
										if err != nil {
											c.AddIssue(fmt.Sprintf("CNAME %s: missing", cname))
										}
									}
								} else {
									c.AddIssue(fmt.Sprintf("Challenge status: %s.", chall.Status.Reason))
								}
							}
						}
					} else {
						auths := []string{}
						for _, auth := range order.Status.Authorizations {
							if auth.Identifier != "" {
								auths = append(auths, auth.Identifier)
							}
						}
						c.AddIssue(fmt.Sprintf("ACME order status: %s. Authorizations: %s, but 0 active challenges.", getAcmeOrderStatus(order), strings.Join(auths, ",")))
					}
				}
			} else {
				statusTrue := false
				statusMessage := ""
				for _, cond := range crs.Status.Conditions {
					if cond.Type == certv1.CertificateRequestConditionReady {
						statusTrue = cond.Status == metav1.ConditionTrue
						statusMessage = cond.Message
						break
					}
				}

				if !statusTrue {
					c.AddIssue(fmt.Sprintf("Certificate request status: %s: %s: %t: %s.", c.C.Name, crs.Name, statusTrue, statusMessage))
				}
			}
		}

	}
	return nil
}

func getAcmeOrderStatus(order *acmev1.Order) string {
	if order.Status.URL == "" {
		return "<unknown order status URL>"
	}

	resp, err := http.DefaultClient.Get(order.Status.URL)
	if err != nil {
		return fmt.Sprintf("<unable to fetch order status: %s", err)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil || len(body) == 0 {
		return fmt.Sprintf("<unable to fetch order status: %s", err)
	}

	var respJson struct {
		Status  int    `json:"status"`
		Detail  string `json:"detail"`
		Expires string `json:"expires"`
	}
	if err := json.Unmarshal(body, &respJson); err != nil {
		return fmt.Sprintf("<unable to fetch order status: %s", err)
	}
	return fmt.Sprintf("%d - %s", respJson.Status, respJson.Detail)
}

func convert(items []certv1.Certificate) []*cert {
	var result []*cert

	for i, _ := range items {
		c := items[i]
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
