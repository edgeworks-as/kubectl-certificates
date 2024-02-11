package internal

import (
	acmev1 "github.com/cert-manager/cert-manager/pkg/apis/acme/v1"
	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	acmeclient "github.com/cert-manager/cert-manager/pkg/client/clientset/versioned/typed/acme/v1"
	certclient "github.com/cert-manager/cert-manager/pkg/client/clientset/versioned/typed/certmanager/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"path/filepath"
)

type Clients interface {
	CurrentNamespace() string
	CertManagerClient() *certclient.CertmanagerV1Client
	AcmeClient() *acmeclient.AcmeV1Client

	GetCertificateRequestForCertificate(cert *certv1.Certificate) (*certv1.CertificateRequest, error)
	GetOrderForCertificateRequest(certificateRequestName string, namespace string) (*acmev1.Order, error)
}

type clients struct {
	config            *rest.Config
	kubeClientSet     *kubernetes.Clientset
	certManagerClient *certclient.CertmanagerV1Client
	acmeClient        *acmeclient.AcmeV1Client
}

func NewClient() (Clients, error) {
	var err error
	k := clients{}

	if k.config, err = getConfig(); err != nil {
		return nil, err
	}

	if k.kubeClientSet, err = getCoreClient(k.config); err != nil {
		return nil, err
	}

	if k.certManagerClient, err = certclient.NewForConfig(k.config); err != nil {
		return nil, err
	}

	if k.acmeClient, err = acmeclient.NewForConfig(k.config); err != nil {
		return nil, err
	}

	return k, nil
}

func getConfig() (*rest.Config, error) {
	return clientcmd.BuildConfigFromFlags("", filepath.Join(homedir.HomeDir(), ".kube", "config"))
}

func getCoreClient(config *rest.Config) (*kubernetes.Clientset, error) {
	return kubernetes.NewForConfig(config)
}

func (c clients) CertManagerClient() *certclient.CertmanagerV1Client {
	return c.certManagerClient
}

func (c clients) AcmeClient() *acmeclient.AcmeV1Client {
	return c.acmeClient
}

func (c clients) CurrentNamespace() string {
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
