package gosocketio

import (
	"errors"
	"reflect"
)

// handler is an event handler representation
type handler struct {
	function         reflect.Value
	arguments        reflect.Type
	argumentsPresent bool
	out              bool
}

var (
	ErrorHandlerNotFunc    = errors.New("f is not a function")
	ErrorHandlerNot2Args   = errors.New("f should have 1 or 2 arguments")
	ErrorHandlerMax1Return = errors.New("f should return no more than one value")
)

// newHandler parses function f using reflection, and stores its representation
func newHandler(f interface{}) (*handler, error) {
	fVal := reflect.ValueOf(f)
	if fVal.Kind() != reflect.Func {
		return nil, ErrorHandlerNotFunc
	}

	fType := fVal.Type()
	if fType.NumOut() > 1 {
		return nil, ErrorHandlerMax1Return
	}

	curCaller := &handler{
		function: fVal,
		out:      fType.NumOut() == 1,
	}

	switch fType.NumIn() {
	case 1:
		curCaller.arguments = nil
		curCaller.argumentsPresent = false
	case 2:
		curCaller.arguments = fType.In(1)
		curCaller.argumentsPresent = true
	default:
		return nil, ErrorHandlerNot2Args
	}

	return curCaller, nil
}

// getArguments returns function parameter as it is present in it using reflection
func (c *handler) getArguments() interface{} { return reflect.New(c.arguments).Interface() }

// callFunc with given arguments from its representation using reflection
func (c *handler) callFunc(h *Channel, arguments interface{}) []reflect.Value {
	// nil is untyped, so use the default empty value of correct type
	if arguments == nil {
		arguments = c.getArguments()
	}

	a := []reflect.Value{reflect.ValueOf(h), reflect.ValueOf(arguments).Elem()}
	if !c.argumentsPresent {
		a = a[0:1]
	}

	return c.function.Call(a)
}
