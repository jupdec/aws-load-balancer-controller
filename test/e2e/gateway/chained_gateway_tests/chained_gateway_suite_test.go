//go:build integration

package chained_gateway_tests

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestChainedGateway(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Chained Gateway Suite")
}

var _ = BeforeSuite(func() {
	Expect(InitTF()).To(Succeed())
})
