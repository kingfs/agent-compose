package qjs

import (
	"context"
	"fmt"
	"strings"

	"github.com/fastschema/qjs"
)

type loaderRegistration struct {
	trigger  LoaderTrigger
	callback *qjs.Value
	order    int
}

type loaderExecutionState struct {
	ctx           context.Context
	host          LoaderHost
	registrations []loaderRegistration
	seenIDs       map[string]struct{}
}

const loaderSchemaBuilderScript = `
(function () {
  function schema(json, validate) {
    return {
      toJSONSchema: function () { return json; },
      parse: function (value) {
        validate(value);
        return value;
      }
    };
  }
  function schemaToJSON(value) {
    if (value && typeof value.toJSONSchema === "function") {
      return value.toJSONSchema();
    }
    return value;
  }
  scheduler.z = {
    string: function () {
      return schema({ type: "string" }, function (value) {
        if (typeof value !== "string") throw new Error("expected string");
      });
    },
    number: function () {
      return schema({ type: "number" }, function (value) {
        if (typeof value !== "number") throw new Error("expected number");
      });
    },
    boolean: function () {
      return schema({ type: "boolean" }, function (value) {
        if (typeof value !== "boolean") throw new Error("expected boolean");
      });
    },
    enum: function (values) {
      return schema({ type: "string", enum: values.slice() }, function (value) {
        if (values.indexOf(value) === -1) throw new Error("expected one of " + values.join(", "));
      });
    },
    array: function (item) {
      return schema({ type: "array", items: schemaToJSON(item) }, function (value) {
        if (!Array.isArray(value)) throw new Error("expected array");
        if (item && typeof item.parse === "function") {
          for (const entry of value) item.parse(entry);
        }
      });
    },
    object: function (shape) {
      const properties = {};
      const required = [];
      for (const key of Object.keys(shape || {})) {
        properties[key] = schemaToJSON(shape[key]);
        required.push(key);
      }
      return schema({ type: "object", properties, required, additionalProperties: false }, function (value) {
        if (!value || typeof value !== "object" || Array.isArray(value)) throw new Error("expected object");
        for (const key of required) {
          if (!(key in value)) throw new Error("missing required property " + key);
          if (shape[key] && typeof shape[key].parse === "function") shape[key].parse(value[key]);
        }
        for (const key of Object.keys(value)) {
          if (!(key in properties)) throw new Error("unexpected property " + key);
        }
      });
    }
  };
})();
`

func installLoaderSchemaBuilder(jsctx *qjs.Context) error {
	value, err := jsctx.Eval("scheduler-z.js", qjs.Code(loaderSchemaBuilderScript))
	if err != nil {
		return fmt.Errorf("install scheduler.z schema builder: %w", err)
	}
	if value != nil {
		value.Free()
	}
	return nil
}

func (e *QJSLoaderEngine) executeRequestedHandler(jsctx *qjs.Context, state *loaderExecutionState, trigger *LoaderTrigger, payload *qjs.Value) (*qjs.Value, error) {
	global := jsctx.Global()
	if trigger != nil && strings.TrimSpace(trigger.ID) != "" {
		for _, registration := range state.registrations {
			if registration.trigger.ID != strings.TrimSpace(trigger.ID) {
				continue
			}
			return jsctx.Invoke(registration.callback, global, payload)
		}
		return nil, fmt.Errorf("loader trigger %s not found in script", strings.TrimSpace(trigger.ID))
	}

	mainFn := global.GetPropertyStr("main")
	if mainFn.IsFunction() {
		return jsctx.Invoke(mainFn, global, payload)
	}

	if len(state.registrations) == 1 {
		return jsctx.Invoke(state.registrations[0].callback, global, payload)
	}
	if len(state.registrations) > 1 {
		return nil, fmt.Errorf("loader defines multiple triggers; choose a trigger explicitly or define main()")
	}
	return jsctx.NewUndefined(), nil
}

func (s *loaderExecutionState) register(trigger LoaderTrigger, callback *qjs.Value) error {
	if _, ok := s.seenIDs[trigger.ID]; ok {
		return fmt.Errorf("duplicate loader trigger id %q", trigger.ID)
	}
	cloned := callback.Clone()
	s.seenIDs[trigger.ID] = struct{}{}
	s.registrations = append(s.registrations, loaderRegistration{
		trigger:  trigger,
		callback: cloned,
		order:    len(s.registrations),
	})
	return nil
}

func (s *loaderExecutionState) unregister(id string, kinds ...string) bool {
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	allowedKinds := make(map[string]struct{}, len(kinds))
	for _, kind := range kinds {
		allowedKinds[strings.TrimSpace(kind)] = struct{}{}
	}
	next := make([]loaderRegistration, 0, len(s.registrations))
	removed := false
	for _, item := range s.registrations {
		if item.trigger.ID == id {
			if len(allowedKinds) == 0 {
				removed = true
				item.callback = nil
				continue
			}
			if _, ok := allowedKinds[item.trigger.Kind]; ok {
				removed = true
				item.callback = nil
				continue
			}
		}
		item.order = len(next)
		next = append(next, item)
	}
	if removed {
		delete(s.seenIDs, id)
		s.registrations = next
	}
	return removed
}

func (s *loaderExecutionState) freeCallbacks() {
	for i := range s.registrations {
		s.registrations[i].callback = nil
	}
}

func (s *loaderExecutionState) triggers() []LoaderTrigger {
	items := make([]LoaderTrigger, 0, len(s.registrations))
	for _, item := range s.registrations {
		items = append(items, item.trigger)
	}
	return items
}
