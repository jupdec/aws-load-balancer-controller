package gateway

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/addon"
	"sigs.k8s.io/aws-load-balancer-controller/v3/pkg/gateway/constants"
	"sigs.k8s.io/controller-runtime/pkg/client"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

// ALBAddons / NLBAddons enumerate the AWS addons each Gateway LB type supports.
// Exported so downstream distributions can reuse the split without duplicating
// the list.
var (
	ALBAddons = []addon.Addon{addon.WAFv2, addon.Shield, addon.ProvisionedCapacity}
	NLBAddons = []addon.Addon{addon.ProvisionedCapacity}
)

const (
	trueString  = "true"
	falseString = "false"
)

// GetStoredAddonConfig parses the addon configuration stored in a Gateway's
// annotations into their representation in native Go structs. Exported so
// downstream distributions can read the same state without maintaining a
// shadow copy.
func GetStoredAddonConfig(gateway *gwv1.Gateway, logger logr.Logger) []addon.AddonMetadata {
	res := make([]addon.AddonMetadata, 0)

	if gateway.Annotations == nil {
		return res
	}

	for annotationKey, annotationValue := range gateway.Annotations {
		if strings.HasPrefix(annotationKey, constants.GatewayLBPrefixEnabledAddon) {
			for _, ao := range addon.AllAddons {
				if annotationKey == GenerateAddOnKey(ao) {
					res = append(res, addon.AddonMetadata{
						Name:    ao,
						Enabled: ParseAddOnEnabledValue(annotationValue, logger),
					})
				}
			}

		}
	}

	return res
}

// GenerateAddOnKey translates an addon into the respective annotation key value.
func GenerateAddOnKey(a addon.Addon) string {
	return fmt.Sprintf("%s%s", constants.GatewayLBPrefixEnabledAddon, strings.ToLower(string(a)))
}

// ParseAddOnEnabledValue parses an annotation key value into a boolean, assuming false if the value is malformed.
func ParseAddOnEnabledValue(e string, logger logr.Logger) bool {
	b, err := strconv.ParseBool(e)
	if err != nil {
		logger.V(1).Info("Unknown boolean value, default it to false", "val", e)
		return false
	}
	return b
}

// StoredAddonNames returns the set of addons whose annotation on the Gateway says Enabled=true.
func StoredAddonNames(gateway *gwv1.Gateway, logger logr.Logger) []addon.Addon {
	res := make([]addon.Addon, 0)
	for _, meta := range GetStoredAddonConfig(gateway, logger) {
		if meta.Enabled {
			res = append(res, meta.Name)
		}
	}
	return res
}

// DiffAddOns determines the additions and subtractions when comparing old (the previous reconcile run result) and new (the current reconcile run result)
func DiffAddOns(old []addon.Addon, new []addon.AddonMetadata) (sets.Set[addon.Addon], sets.Set[addon.Addon]) {
	additions := sets.New[addon.Addon]()
	removals := sets.New[addon.Addon]()

	oldSet := sets.New(old...)
	newSet := sets.New[addon.Addon]()

	for _, newItem := range new {
		if newItem.Enabled {
			newSet.Insert(newItem.Name)
		}

		if !oldSet.Has(newItem.Name) && newItem.Enabled {
			additions.Insert(newItem.Name)
		}

	}

	for _, aOld := range old {
		if !newSet.Has(aOld) {
			removals.Insert(aOld)
		}
	}

	return additions, removals
}

// PersistAddOns persists the enabled/disabled addons to the Gateway annotations.
// This assumes that changes is the complete set of addons whose annotation should
// be set (to trueString when remove=false, falseString when remove=true).
func PersistAddOns(ctx context.Context, k8sClient client.Client, gw *gwv1.Gateway, changes []addon.Addon, remove bool) error {
	annotations := make(map[string]string)
	if gw.Annotations != nil {
		for k, v := range gw.Annotations {
			annotations[k] = v
		}
	}

	var annotationValue = trueString
	if remove {
		annotationValue = falseString
	}

	for _, ao := range changes {
		annotations[GenerateAddOnKey(ao)] = annotationValue
	}

	gwOld := gw.DeepCopy()
	gw.Annotations = annotations
	return k8sClient.Patch(ctx, gw, client.MergeFrom(gwOld))
}
