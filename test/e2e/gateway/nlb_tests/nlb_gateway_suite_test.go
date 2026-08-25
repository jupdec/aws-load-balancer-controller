package nlb_tests

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestNLBGateway(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "NLB Gateway Suite")
}

var _ = BeforeSuite(func() {
	Expect(InitTF()).To(Succeed())
})
