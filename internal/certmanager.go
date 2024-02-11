package internal

import (
	"context"
	acmev1 "github.com/cert-manager/cert-manager/pkg/apis/acme/v1"
	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (c clients) GetCertificateRequestForCertificate(cert *certv1.Certificate) (*certv1.CertificateRequest, error) {
	// Find certificate request matching the cert
	crslist, err := c.CertManagerClient().CertificateRequests(cert.Namespace).List(context.Background(), v1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var found *certv1.CertificateRequest
	for _, crs := range crslist.Items {
		if len(crs.OwnerReferences) == 0 {
			continue
		}

		if crs.OwnerReferences[0].Kind != "Certificate" {
			continue
		}

		if crs.OwnerReferences[0].Name == cert.Name && crs.OwnerReferences[0].UID == cert.UID {
			if found == nil || crs.ObjectMeta.CreationTimestamp.After(found.ObjectMeta.CreationTimestamp.Time) {
				found = &crs
			}
		}
	}
	return found, nil
}

func (c clients) GetOrderForCertificateRequest(certificateRequestName string, namespace string) (*acmev1.Order, error) {
	orderList, err := c.AcmeClient().Orders(namespace).List(context.Background(), v1.ListOptions{})
	if err != nil {
		return nil, err
	}

	var found *acmev1.Order
	for _, order := range orderList.Items {
		if len(order.OwnerReferences) == 0 {
			continue
		}

		if order.OwnerReferences[0].Kind != "CertificateRequest" || order.OwnerReferences[0].Name != certificateRequestName {
			continue
		}

		if found == nil || order.ObjectMeta.CreationTimestamp.After(found.CreationTimestamp.Time) {
			found = &order
		}
	}
	return found, nil
}
