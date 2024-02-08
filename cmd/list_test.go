package cmd

import (
	"fmt"
	certv1 "github.com/cert-manager/cert-manager/pkg/apis/certmanager/v1"
	v1 "github.com/cert-manager/cert-manager/pkg/apis/meta/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"testing"
	"time"
)

func Test_sortCerts(t *testing.T) {

	type args struct {
		certList             []certv1.Certificate
		sortName             bool
		sortReady            bool
		sortIssuer           bool
		sortFrom             bool
		sortTo               bool
		expectedOrderOfNames []string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "sort-name",
			args: args{
				certList:             createCerts(),
				sortName:             true,
				sortReady:            false,
				sortIssuer:           false,
				sortFrom:             false,
				sortTo:               false,
				expectedOrderOfNames: []string{"a", "b", "c"},
			},
		},
		{
			name: "sort-issuer",
			args: args{
				certList:             createCerts(),
				sortName:             false,
				sortReady:            false,
				sortIssuer:           true,
				sortFrom:             false,
				sortTo:               false,
				expectedOrderOfNames: []string{"a", "b", "c"},
			},
		},
		{
			name: "sort-issuer",
			args: args{
				certList:             createCerts(),
				sortName:             false,
				sortReady:            false,
				sortIssuer:           true,
				sortFrom:             false,
				sortTo:               false,
				expectedOrderOfNames: []string{"a", "b", "c"},
			},
		},
		{
			name: "sort-from",
			args: args{
				certList:             createCerts(),
				sortName:             false,
				sortReady:            false,
				sortIssuer:           false,
				sortFrom:             true,
				sortTo:               false,
				expectedOrderOfNames: []string{"a", "b", "c"},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sortCerts(tt.args.certList, tt.args.sortName, tt.args.sortReady, tt.args.sortIssuer, tt.args.sortFrom, tt.args.sortTo)
			if len(tt.args.certList) != len(tt.args.expectedOrderOfNames) {
				t.Error(fmt.Sprintf("sorted list length not equals to expected names list: %d != %d", len(tt.args.certList), len(tt.args.expectedOrderOfNames)))
			}
			for i, n := range tt.args.expectedOrderOfNames {
				if n != tt.args.certList[i].Name {
					t.Error(fmt.Errorf("element %d name %s != expected name %s", i, tt.args.certList[9].Name, n))
				}
				fmt.Printf("Cert: %s\n", tt.args.certList[i].Name)
			}
		})
	}
}

func createCerts() []certv1.Certificate {
	return []certv1.Certificate{
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "c",
			},
			Spec: certv1.CertificateSpec{
				IssuerRef: v1.ObjectReference{
					Name: "issuerc",
				},
			},
			Status: certv1.CertificateStatus{
				NotBefore: createTime(-3),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "b",
			},
			Spec: certv1.CertificateSpec{
				IssuerRef: v1.ObjectReference{
					Name: "issuerb",
				},
			},
			Status: certv1.CertificateStatus{
				NotBefore: createTime(-2),
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{
				Name: "a",
			},
			Spec: certv1.CertificateSpec{
				IssuerRef: v1.ObjectReference{
					Name: "issuera",
				},
			},
			Status: certv1.CertificateStatus{
				NotBefore: createTime(-1),
			},
		},
	}
}

func createTime(sinceDays int) *metav1.Time {
	mt := metav1.NewTime(time.Now().Add(time.Hour * time.Duration(-sinceDays) * 24))
	return &mt
}
