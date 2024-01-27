// Package dto is an easy-to-use library for data mapping.
//
// dto maps primitives, structs, slices, maps, pointers
// and supports custom functions and error mapping.
//
// Contrary to other struct mappers it uses only name based field resolution
// and maps its values recursively. This means that go-dto tries to map struct fields
// with the same names.
//
// Conversion functions can be used to overwrite mapping behaviour.
// Inspection functions allow to modify a value after it has been mapped.
//
// See the tests and github page for more exmaples.
package dto

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

type structValueMap = map[string]reflect.Value

// Marker type for functions with no receiver
type nilRecvT struct{}

var nilRecvRfType = reflect.TypeOf(nilRecvT{})
var errorRfType = reflect.TypeOf((*error)(nil)).Elem()
var mapperPtrRfType = reflect.TypeOf((*Mapper)(nil))

type convertFuncClosure = func(reflect.Value, *Mapper) (reflect.Value, error)
type inspectFuncClosure = func(reflect.Value, reflect.Value, *Mapper) error

const structTag = "dto"

// NoValidMappingError indicates that no valid mapping was found
type NoValidMappingError struct {
	ToType   reflect.Type
	FromType reflect.Type
}

func (nvme NoValidMappingError) Error() string {
	return fmt.Sprintf("No valid mapping found for %v from %v", nvme.ToType, nvme.FromType)
}

// Mapper contains conversion and inspect functions
type Mapper struct {
	// linear search might be faster than nested maps
	convFunc map[reflect.Type]map[reflect.Type]convertFuncClosure
	postFunc map[reflect.Type]map[reflect.Type][]inspectFuncClosure
}

// ==================================== utils =================================

// Collect all struct fields (including anonymous) into a structValueMap
func collectStructFields(rfValue reflect.Value, rfType reflect.Type, fields structValueMap) {
	for i := 0; i < rfType.NumField(); i++ {
		fieldValue := rfValue.Field(i)
		fieldType := rfType.Field(i)
		if tags, ok := fieldType.Tag.Lookup(structTag); ok {
			if strings.Contains(tags, "ignore") {
				continue
			}
		}
		if fieldType.Anonymous {
			collectStructFields(fieldValue, fieldType.Type, fields)
		} else {
			fields[fieldType.Name] = fieldValue
		}
	}
}

// Return reflect.Value with pointer removed (first layer only)
func reflectValueRemovePtr(v interface{}) reflect.Value {
	rv := reflect.ValueOf(v)
	if rv.Type().Kind() == reflect.Ptr {
		return rv.Elem()
	}
	return rv
}

// Maps an error from a reflect value
// Panics if the value is non nill and not an error
func errorFromReflectValue(rv reflect.Value) error {
	if rv.IsNil() {
		return nil
	}
	err, ok := rv.Interface().(error)
	if !ok {
		panic("Failed to map error from reflect.Value")
	}
	return err
}

// ==================================== Conversion and inspection functions ===

// Run inspect functions for (dst-src) pair
func (m *Mapper) runInspectFuncs(dstRv, srcRv reflect.Value) error {
	toMap, ok := m.postFunc[dstRv.Type()]
	if !ok {
		return nil
	}
	for _, recvType := range []reflect.Type{srcRv.Type(), nilRecvRfType} {
		funcs, ok := toMap[recvType]
		if !ok {
			continue
		}
		for _, fun := range funcs {
			if err := fun(dstRv.Addr(), srcRv, m); err != nil {
				return err
			}
		}
	}
	return nil
}

// Run convert function for (dst-src) pair
// Returns (error, true) if a valid function was found, (nil, false) otherwise
func (m *Mapper) runConvFuncs(dstRv, srcRv reflect.Value) (bool, error) {
	toMap, ok := m.convFunc[srcRv.Type()]
	if !ok {
		return false, nil
	}
	if convertFunc, ok := toMap[dstRv.Type()]; ok {
		val, err := convertFunc(srcRv, m)
		if err != nil {
			return true, err
		}
		dstRv.Set(val)
		return true, nil
	}
	return false, nil
}

// HasCustomFuncs returns true if the Mapper has custom functions defined
func (m *Mapper) HasCustomFuncs() bool {
	return len(m.convFunc)+len(m.postFunc) > 0
}

// AddConvFunc adds a conversion function to the Mapper
//
// Panics if f is not a valid conversion function
// Overwrites previous functions with the same type pair
func (m *Mapper) AddConvFunc(f interface{}) {
	rt := reflect.TypeOf(f)

	// check basic argument invariant
	if rt.NumOut() < 1 || rt.NumIn() < 1 {
		panic("Bad conversion function")
	}

	// check if to inject mapper
	takesMapper := false
	if rt.NumIn() > 1 && rt.In(1) == mapperPtrRfType {
		takesMapper = true
	}

	// check if returns an error
	returnsError := false
	outType := rt.Out(0)
	if rt.NumOut() > 1 && rt.Out(1).Implements(errorRfType) {
		returnsError = true
	}

	inType := rt.In(0)

	// create maps
	if len(m.convFunc) == 0 {
		m.convFunc = make(map[reflect.Type]map[reflect.Type]convertFuncClosure)
	}
	if len(m.convFunc[inType]) == 0 {
		m.convFunc[inType] = make(map[reflect.Type]convertFuncClosure)
	}

	// register closure
	m.convFunc[inType][outType] = func(from reflect.Value, m *Mapper) (reflect.Value, error) {
		args := []reflect.Value{from}
		if takesMapper {
			args = append(args, reflect.ValueOf(m))
		}
		out := reflect.ValueOf(f).Call(args)
		if returnsError {
			return out[0], errorFromReflectValue(out[1])
		}
		return out[0], nil
	}
}

// AddInspectFunc adds an inspection function to the Mapper
//
// Panics if f is not a valid inspection function
func (m *Mapper) AddInspectFunc(f interface{}) {
	ft := reflect.TypeOf(f)
	inType := ft.In(0).Elem()

	// check if takes from
	fromType := nilRecvRfType
	if ft.NumIn() > 1 {
		fromType = ft.In(1)
	}

	// check if takes mapper
	takesMapper := false
	if ft.NumIn() > 2 && ft.In(2) == reflect.TypeOf(m) {
		takesMapper = true
	}

	// check if returns error
	returnsError := false
	if ft.NumOut() > 0 && ft.Out(0).Implements(errorRfType) {
		returnsError = true
	}

	// create map path
	if len(m.postFunc) == 0 {
		m.postFunc = make(map[reflect.Type]map[reflect.Type][]inspectFuncClosure)
	}
	if len(m.postFunc[inType]) == 0 {
		m.postFunc[inType] = make(map[reflect.Type][]inspectFuncClosure)
	}

	// register closure
	m.postFunc[inType][fromType] = append(m.postFunc[inType][fromType],
		func(v1, v2 reflect.Value, m *Mapper) error {
			args := []reflect.Value{v1}
			if fromType != nilRecvRfType {
				args = append(args, v2)
			}
			if takesMapper {
				args = append(args, reflect.ValueOf(m))
			}

			out := reflect.ValueOf(f).Call(args)
			if returnsError {
				return errorFromReflectValue(out[0])
			}
			return nil
		},
	)
}

// ==================================== Mapping functions =====================

// Map slices
// Panics if arguments are not slices
func (m *Mapper) mapSlice(toRv, fromRv reflect.Value) error {
	toRv.Set(reflect.MakeSlice(toRv.Type(), fromRv.Len(), fromRv.Len()))
	for i := 0; i < fromRv.Len(); i++ {
		if err := m.mapValue(toRv.Index(i), fromRv.Index(i)); err != nil {
			return err
		}
	}
	return nil
}

// Map maps
// Panics if arguments are not maps
func (m *Mapper) mapMap(dstRv, srcRv reflect.Value) error {
	dstRv.Set(reflect.MakeMapWithSize(dstRv.Type(), srcRv.Len()))
	// Map values
	mapIt := srcRv.MapRange()
	for mapIt.Next() {
		toKey := reflect.New(dstRv.Type().Key()).Elem()
		toValue := reflect.New(dstRv.Type().Elem()).Elem()
		if err := m.mapValue(toKey, mapIt.Key()); err != nil {
			return err
		}
		if err := m.mapValue(toValue, mapIt.Value()); err != nil {
			return err
		}
		dstRv.SetMapIndex(toKey, toValue)
	}
	return nil
}

// Map structs
// Panics if arguments are not structs
func (m *Mapper) mapStructs(dstRv, srcRv reflect.Value) error {
	toFields := make(structValueMap)
	collectStructFields(dstRv, dstRv.Type(), toFields)

	fromFields := make(structValueMap)
	collectStructFields(srcRv, srcRv.Type(), fromFields)

	for fieldName, toValue := range toFields {
		fromValue, ok := fromFields[fieldName]
		if !ok {
			continue
		}
		err := m.mapValue(toValue, fromValue)
		if err != nil {
			return err
		}
	}

	return nil
}

// Map map values to slice
// Panics if arguments are not slice and map accordingly
func (m *Mapper) mapMapToSlice(dstRv, srcRv reflect.Value) error {
	dstRv.Set(reflect.MakeSlice(dstRv.Type(), srcRv.Len(), srcRv.Len()))
	i := 0
	mapIt := srcRv.MapRange()
	for mapIt.Next() {
		if err := m.mapValue(dstRv.Index(i), mapIt.Value()); err != nil {
			return err
		}
		i++
	}
	return nil
}

// Map a map of slices to slice
// Panics of arguments are not a map of slices and a slice accordingly
func (m *Mapper) mapMapSlicesToSlice(dstRv, srcRv reflect.Value) error {
	// calculate length
	sumLen := 0
	mapIt := srcRv.MapRange()
	for mapIt.Next() {
		sumLen += mapIt.Value().Len()
	}

	dstRv.Set(reflect.MakeSlice(dstRv.Type(), sumLen, sumLen))

	i := 0
	mapIt = srcRv.MapRange()
	for mapIt.Next() {
		mapSlice := mapIt.Value()
		for j := 0; j < mapSlice.Len(); i, j = i+1, j+1 {
			if err := m.mapValue(dstRv.Index(i), mapSlice.Index(j)); err != nil {
				return err
			}
		}
	}

	return nil
}

// Try to map any value
func (m *Mapper) mapValue(dstRv, srcRv reflect.Value) (returnError error) {
	tk, fk := dstRv.Type().Kind(), srcRv.Type().Kind()

	// Defer inspect functions
	defer func() {
		if returnError != nil {
			return
		}
		returnError = m.runInspectFuncs(dstRv, srcRv)
	}()

	// 1. Check conversion functions
	converted, err := m.runConvFuncs(dstRv, srcRv)
	if converted {
		return err
	}

	// 2. Check direct assignment
	if srcRv.Type().AssignableTo(dstRv.Type()) {
		dstRv.Set(srcRv)
		return
	}

	// 3. Check conversion
	if srcRv.Type().ConvertibleTo(dstRv.Type()) {
		dstRv.Set(srcRv.Convert(dstRv.Type()))
		return
	}

	// 4. Handle pointers by dereferencing from
	if fk == reflect.Ptr {
		// Skip null pointers
		if srcRv.IsNil() {
			return nil
		}
		return m.mapValue(dstRv, srcRv.Elem())
	}

	// 5. Handle pointers by dereferencing to
	if tk == reflect.Ptr {
		// Allocate new value if nil
		if dstRv.IsNil() {
			dstRv.Set(reflect.New(dstRv.Type().Elem()))
		}
		return m.mapValue(dstRv.Elem(), srcRv)
	}

	// 6. Handle sructs
	if tk == reflect.Struct && fk == reflect.Struct {
		return m.mapStructs(dstRv, srcRv)
	}

	// 7. Handle slices
	if tk == reflect.Slice && fk == reflect.Slice {
		return m.mapSlice(dstRv, srcRv)
	}

	// 8. Handle maps
	if tk == reflect.Map && fk == reflect.Map {
		return m.mapMap(dstRv, srcRv)
	}

	// 9. Handle map to slice
	if tk == reflect.Slice && fk == reflect.Map {
		err := m.mapMapToSlice(dstRv, srcRv)

		// 9. Handle map of slices to slice
		mapElemK := srcRv.Type().Elem().Kind()
		if errors.As(err, &NoValidMappingError{}) && mapElemK == reflect.Slice {
			// dont propagate errors
			if errFlatten := m.mapMapSlicesToSlice(dstRv, srcRv); errFlatten == nil {
				return
			}
		}

		return err
	}

	return NoValidMappingError{
		ToType:   dstRv.Type(),
		FromType: srcRv.Type(),
	}
}

// ==================================== Public helpers ========================

// Map transfers values from src to dst
func (m *Mapper) Map(dst, src interface{}) error {
	return m.mapValue(reflectValueRemovePtr(dst), reflectValueRemovePtr(src))
}

// Map transfers values from src to dst
func Map(dst, src interface{}) error {
	m := Mapper{}
	return m.Map(dst, src)
}
