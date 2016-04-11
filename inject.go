// Package inject provides utilities for mapping and injecting dependencies.
//
// Fork of [codegangsta's inject](https://github.com/codegangsta/inject)
// since it seems to be unmaintained.
package inject

import (
	"fmt"
	"reflect"
)

// Injector represents an interface for mapping and injecting dependencies into structs
// and function arguments.
type Injector interface {
	// SetParent sets the parent of the injector. If the injector cannot find a
	// dependency in its Type map it will check its parent before returning an
	// error.
	SetParent(Injector)

	// ApplyMap applies dependencies to the provided struct and registers it
	// if it is successful.
	ApplyMap(interface{}) (Injector, error)

	// Maps dependencies in the Type map to each field in the struct
	// that is tagged with 'inject'. Returns an error if the injection
	// fails.
	Apply(interface{}) error

	// Invoke attempts to call the interface{} provided as a function,
	// providing dependencies for function arguments based on Type. Returns
	// a slice of reflect.Value representing the returned values of the function.
	// Returns an error if the injection fails.
	Invoke(interface{}) ([]reflect.Value, error)

	// Maps the interface{} value based on its immediate type from reflect.TypeOf.
	Map(interface{}) Injector

	// Maps the interface{} value based on the pointer of an Interface provided.
	// This is really only useful for mapping a value as an interface, as interfaces
	// cannot at this time be referenced directly without a pointer.
	MapTo(interface{}, interface{}) Injector

	// Provide the dynamic type of interface{} returns.
	Provide(interface{}) Injector

	// Provides a possibility to directly insert a mapping based on type and value.
	// This makes it possible to directly map type arguments not possible to instantiate
	// with reflect like unidirectional channels.
	Set(reflect.Type, reflect.Value) Injector

	// Returns the Value that is mapped to the current type. Returns a zeroed Value if
	// the Type has not been mapped.
	Get(reflect.Type) reflect.Value
}

type injector struct {
	values    map[reflect.Type]reflect.Value
	providers map[reflect.Type]reflect.Value
	parent    Injector
}

// InterfaceOf dereferences a pointer to an Interface type.
// It panics if value is not an pointer to an interface.
func InterfaceOf(value interface{}) reflect.Type {
	t := reflect.TypeOf(value)

	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Interface {
		panic("Called inject.InterfaceOf with a value that is not a pointer to an interface. (*MyInterface)(nil)")
	}

	return t
}

// New returns a new Injector.
func New() Injector {
	return &injector{
		values:    make(map[reflect.Type]reflect.Value),
		providers: make(map[reflect.Type]reflect.Value),
	}
}

// Invoke attempts to call the interface{} provided as a function,
// providing dependencies for function arguments based on Type.
// Returns a slice of reflect.Value representing the returned values of the function.
// Returns an error if the injection fails.
// It panics if f is not a function
func (inj *injector) Invoke(f interface{}) ([]reflect.Value, error) {
	t := reflect.TypeOf(f)

	var in = make([]reflect.Value, t.NumIn()) //Panic if t is not kind of Func
	for i := 0; i < t.NumIn(); i++ {
		argType := t.In(i)
		val := inj.Get(argType)
		if !val.IsValid() {
			return nil, fmt.Errorf("Value not found for type %v", argType)
		}

		in[i] = val
	}

	return reflect.ValueOf(f).Call(in), nil
}

// Maps dependencies in the Type map to each field in the struct
// that is tagged with 'inject'.
// Returns an error if the injection fails.
func (inj *injector) Apply(val interface{}) error {
	v := reflect.ValueOf(val)

	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		return nil // Should not panic here ?
	}

	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		structField := t.Field(i)
		if f.CanSet() && (structField.Tag == "inject" || structField.Tag.Get("inject") != "") {
			ft := f.Type()
			v := inj.Get(ft)
			if !v.IsValid() {
				return fmt.Errorf("Value not found for type %v", ft)
			}

			f.Set(v)
		}

	}

	return nil
}

// Maps the concrete value of val to its dynamic type using reflect.TypeOf,
// It returns the Injector registered in.
func (i *injector) Map(val interface{}) Injector {
	i.values[reflect.TypeOf(val)] = reflect.ValueOf(val)
	return i
}

// Applies dependencies to the struct then Maps it if it is successful.
func (i *injector) ApplyMap(val interface{}) (Injector, error) {
	err := i.Apply(val)
	if err != nil {
		return nil, err
	}
	return i.Map(val), nil
}

func (i *injector) MapTo(val interface{}, ifacePtr interface{}) Injector {
	i.values[InterfaceOf(ifacePtr)] = reflect.ValueOf(val)
	return i
}

// Provide the dynamic type of provider returns,
// It returns the TypeMapper registered in.
func (inj *injector) Provide(provider interface{}) Injector {
	val := reflect.ValueOf(provider)
	t := val.Type()
	numout := t.NumOut()
	for i := 0; i < numout; i++ {
		out := t.Out(i)
		inj.providers[out] = val
	}
	return inj
}

// Maps the given reflect.Type to the given reflect.Value and returns
// the Typemapper the mapping has been registered in.
func (i *injector) Set(typ reflect.Type, val reflect.Value) Injector {
	i.values[typ] = val
	return i
}

func (i *injector) Get(t reflect.Type) reflect.Value {
	val := i.values[t]

	if val.IsValid() {
		return val
	}

	// try to find providers
	if provider, ok := i.providers[t]; ok {
		// invoke provider to inject return values
		results, err := i.Invoke(provider.Interface())
		if err != nil {
			panic(err)
		}
		for _, result := range results {
			resultType := result.Type()

			i.values[resultType] = result

			// provider should not be called again
			delete(i.providers, resultType)

			if resultType == t {
				val = result
			}
		}
		if val.IsValid() {
			return val
		}
	}

	// no concrete types found, try to find implementors
	// if t is an interface
	if t.Kind() == reflect.Interface {
		for k, v := range i.values {
			if k.Implements(t) && v.IsValid() {
				return v
			}
		}
	}

	// Still no type found, try to look it up on the parent
	if i.parent != nil {
		val = i.parent.Get(t)
	}

	return val

}

func (i *injector) SetParent(parent Injector) {
	i.parent = parent
}
