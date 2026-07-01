package runtime

import (
	"agent-compose/pkg/agentcompose/session"
	driverpkg "agent-compose/pkg/driver"
)

func toDriverSession(item *session.Session) *driverpkg.Session {
	if item == nil {
		return nil
	}
	envItems := make([]driverpkg.SessionEnvVar, 0, len(item.EnvItems))
	for _, env := range item.EnvItems {
		envItems = append(envItems, driverpkg.SessionEnvVar{Name: env.Name, Value: env.Value, Secret: env.Secret})
	}
	runtimeEnvItems := make([]driverpkg.SessionEnvVar, 0, len(item.RuntimeEnvItems))
	for _, env := range item.RuntimeEnvItems {
		runtimeEnvItems = append(runtimeEnvItems, driverpkg.SessionEnvVar{Name: env.Name, Value: env.Value, Secret: env.Secret})
	}
	return &driverpkg.Session{
		Summary: driverpkg.SessionSummary{
			ID:            item.Summary.ID,
			Driver:        item.Summary.Driver,
			GuestImage:    item.Summary.GuestImage,
			RuntimeRef:    item.Summary.RuntimeRef,
			WorkspacePath: item.Summary.WorkspacePath,
			CreatedAt:     item.Summary.CreatedAt,
			UpdatedAt:     item.Summary.UpdatedAt,
		},
		EnvItems:        envItems,
		RuntimeEnvItems: runtimeEnvItems,
	}
}

func toDriverVMState(state session.VMState) driverpkg.VMState {
	return driverpkg.VMState{
		Driver:       state.Driver,
		Mode:         state.Mode,
		BoxName:      state.BoxName,
		BoxID:        state.BoxID,
		Image:        state.Image,
		Registry:     state.Registry,
		RuntimeHome:  state.RuntimeHome,
		StartedAt:    state.StartedAt,
		StoppedAt:    state.StoppedAt,
		LastError:    state.LastError,
		BootstrapRef: state.BootstrapRef,
	}
}

func fromDriverVMState(state driverpkg.VMState) session.VMState {
	return session.VMState{
		Driver:       state.Driver,
		Mode:         state.Mode,
		BoxName:      state.BoxName,
		BoxID:        state.BoxID,
		Image:        state.Image,
		Registry:     state.Registry,
		RuntimeHome:  state.RuntimeHome,
		StartedAt:    state.StartedAt,
		StoppedAt:    state.StoppedAt,
		LastError:    state.LastError,
		BootstrapRef: state.BootstrapRef,
	}
}
