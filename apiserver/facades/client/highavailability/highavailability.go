// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability

import (
	"sort"
	"strconv"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/constraints"
	"github.com/juju/juju/controller"
	"github.com/juju/juju/mongo"
	"github.com/juju/juju/network"
	"github.com/juju/juju/permission"
	"github.com/juju/juju/state"
)

var logger = loggo.GetLogger("juju.apiserver.highavailability")

// HighAvailability defines the methods on the highavailability API end point.
type HighAvailability interface {
	EnableHA(args params.ControllersSpecs) (params.ControllersChangeResults, error)
}

// HighAvailabilityAPI implements the HighAvailability interface and is the concrete
// implementation of the api end point.
type HighAvailabilityAPI struct {
	state      *state.State
	resources  facade.Resources
	authorizer facade.Authorizer

	// machineID is the ID of the machine where the API server is running.
	machineID string
}

var _ HighAvailability = (*HighAvailabilityAPI)(nil)

// NewHighAvailabilityAPI creates a new server-side highavailability API end point.
func NewHighAvailabilityAPI(st *state.State, resources facade.Resources, authorizer facade.Authorizer) (*HighAvailabilityAPI, error) {
	// Only clients can access the high availability facade.
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}
	machineID, err := extractResourceValue(resources, "machineID")
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &HighAvailabilityAPI{
		state:      st,
		resources:  resources,
		authorizer: authorizer,
		machineID:  machineID,
	}, nil
}

func extractResourceValue(resources facade.Resources, key string) (string, error) {
	res := resources.Get(key)
	strRes, ok := res.(common.StringResource)
	if !ok {
		if res == nil {
			strRes = ""
		} else {
			return "", errors.Errorf("invalid %s resource: %v", key, res)
		}
	}
	return strRes.String(), nil
}

// EnableHA adds controller machines as necessary to ensure the
// controller has the number of machines specified.
func (api *HighAvailabilityAPI) EnableHA(args params.ControllersSpecs) (params.ControllersChangeResults, error) {
	results := params.ControllersChangeResults{}

	admin, err := api.authorizer.HasPermission(permission.SuperuserAccess, api.state.ControllerTag())
	if err != nil && !errors.IsNotFound(err) {
		return results, errors.Trace(err)
	}
	if !admin {
		return results, common.ServerError(common.ErrPerm)
	}

	if len(args.Specs) == 0 {
		return results, nil
	}
	if len(args.Specs) > 1 {
		return results, errors.New("only one controller spec is supported")
	}

	result, err := api.enableHASingle(api.state, args.Specs[0])
	results.Results = make([]params.ControllersChangeResult, 1)
	results.Results[0].Result = result
	results.Results[0].Error = common.ServerError(err)
	return results, nil
}

func (api *HighAvailabilityAPI) enableHASingle(st *state.State, spec params.ControllersSpec) (
	params.ControllersChanges, error,
) {
	if !st.IsController() {
		return params.ControllersChanges{}, errors.New("unsupported with hosted models")
	}
	// Check if changes are allowed and the command may proceed.
	blockChecker := common.NewBlockChecker(st)
	if err := blockChecker.ChangeAllowed(); err != nil {
		return params.ControllersChanges{}, errors.Trace(err)
	}

	cInfo, err := st.ControllerInfo()
	if err != nil {
		return params.ControllersChanges{}, err
	}

	if spec.Series == "" {
		// We should always have at least one voting machine
		// If we *really* wanted we could just pick whatever series is
		// in the majority, but really, if we always copy the value of
		// the first one, then they'll stay in sync.
		if len(cInfo.VotingMachineIds) == 0 {
			// Better than a panic()?
			return params.ControllersChanges{}, errors.Errorf("internal error, failed to find any voting machines")
		}
		templateMachine, err := st.Machine(cInfo.VotingMachineIds[0])
		if err != nil {
			return params.ControllersChanges{}, err
		}
		spec.Series = templateMachine.Series()
	}

	// If there were no supplied constraints, use the original bootstrap
	// constraints.
	if constraints.IsEmpty(&spec.Constraints) {
		var err error
		spec.Constraints, err = getBootstrapConstraints(st, cInfo.MachineIds)
		if err != nil {
			return params.ControllersChanges{}, errors.Trace(err)
		}
	}

	// Retrieve the controller configuration and merge any implied space
	// constraints into the spec constraints.
	cfg, err := st.ControllerConfig()
	if err != nil {
		return params.ControllersChanges{}, errors.Annotate(err, "retrieving controller config")
	}
	if err = validateCurrentControllers(st, cfg, cInfo.MachineIds); err != nil {
		return params.ControllersChanges{}, errors.Trace(err)
	}
	spec.Constraints.Spaces = cfg.AsSpaceConstraints(spec.Constraints.Spaces)

	// Might be nicer to pass the spec itself to this method.
	changes, err := st.EnableHA(spec.NumControllers, spec.Constraints, spec.Series, spec.Placement, api.machineID)
	if err != nil {
		return params.ControllersChanges{}, err
	}
	return controllersChanges(changes), nil
}

// validateCurrentControllers checks for a scenario where there is no HA space
// in controller configuration and more than one machine-local address on any
// of the controller machines. An error is returned if it is detected.
// When HA space is set, there are other code paths that ensure controllers
// have at least one address in the space.
func validateCurrentControllers(st *state.State, cfg controller.Config, machineIds []string) error {
	if cfg.JujuHASpace() != "" {
		return nil
	}

	var badIds []string
	for _, id := range machineIds {
		controller, err := st.Machine(id)
		if err != nil {
			return errors.Annotatef(err, "reading controller id %v", id)
		}
		internal := network.SelectInternalAddresses(controller.Addresses(), false)
		if len(internal) != 1 {
			badIds = append(badIds, id)
		}
	}
	if len(badIds) > 0 {
		return errors.Errorf(
			"juju-ha-space is not set and a unique cloud-local address was not found for machines: %s",
			strings.Join(badIds, ", "),
		)
	}
	return nil
}

// getBootstrapConstraints attempts to return the constraints for the initial
// bootstrapped controller.
func getBootstrapConstraints(st *state.State, machineIds []string) (constraints.Value, error) {
	// Sort the controller IDs from low to high and take the first.
	// This will typically give the initial bootstrap machine.
	var controllerIds []int
	for _, id := range machineIds {
		idNum, err := strconv.Atoi(id)
		if err != nil {
			logger.Warningf("ignoring non numeric controller id %v", id)
			continue
		}
		controllerIds = append(controllerIds, idNum)
	}
	if len(controllerIds) == 0 {
		return constraints.Value{}, errors.Errorf("internal error; failed to find any controllers")
	}
	sort.Ints(controllerIds)
	controllerId := controllerIds[0]

	// Load the controller machine and get its constraints.
	controller, err := st.Machine(strconv.Itoa(controllerId))
	if err != nil {
		return constraints.Value{}, errors.Annotatef(err, "reading controller id %v", controllerId)
	}

	cons, err := controller.Constraints()
	return cons, errors.Annotatef(err, "reading constraints for controller id %v", controllerId)
}

// controllersChanges generates a new params instance from the state instance.
func controllersChanges(change state.ControllersChanges) params.ControllersChanges {
	return params.ControllersChanges{
		Added:      machineIdsToTags(change.Added...),
		Maintained: machineIdsToTags(change.Maintained...),
		Removed:    machineIdsToTags(change.Removed...),
		Promoted:   machineIdsToTags(change.Promoted...),
		Demoted:    machineIdsToTags(change.Demoted...),
		Converted:  machineIdsToTags(change.Converted...),
	}
}

// machineIdsToTags returns a slice of machine tag strings created from the
// input machine IDs.
func machineIdsToTags(ids ...string) []string {
	var result []string
	for _, id := range ids {
		result = append(result, names.NewMachineTag(id).String())
	}
	return result
}

// StopHAReplicationForUpgrade will prompt the HA cluster to enter upgrade
// mongo mode.
func (api *HighAvailabilityAPI) StopHAReplicationForUpgrade(args params.UpgradeMongoParams) (
	params.MongoUpgradeResults, error,
) {
	ha, err := api.state.SetUpgradeMongoMode(mongo.Version{
		Major:         args.Target.Major,
		Minor:         args.Target.Minor,
		Patch:         args.Target.Patch,
		StorageEngine: mongo.StorageEngine(args.Target.StorageEngine),
	})
	if err != nil {
		return params.MongoUpgradeResults{}, errors.Annotate(err, "cannot stop HA for ugprade")
	}
	members := make([]params.HAMember, len(ha.Members))
	for i, m := range ha.Members {
		members[i] = params.HAMember{
			Tag:           m.Tag,
			PublicAddress: m.PublicAddress,
			Series:        m.Series,
		}
	}
	return params.MongoUpgradeResults{
		Master: params.HAMember{
			Tag:           ha.Master.Tag,
			PublicAddress: ha.Master.PublicAddress,
			Series:        ha.Master.Series,
		},
		Members:   members,
		RsMembers: ha.RsMembers,
	}, nil
}

// ResumeHAReplicationAfterUpgrade will add the upgraded members of HA
// cluster to the upgraded master.
func (api *HighAvailabilityAPI) ResumeHAReplicationAfterUpgrade(args params.ResumeReplicationParams) error {
	return api.state.ResumeReplication(args.Members)
}
