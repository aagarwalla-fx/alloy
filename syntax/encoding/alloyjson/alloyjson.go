// Package alloyjson encodes Alloy configuration syntax as JSON.
package alloyjson

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/grafana/alloy/syntax/internal/reflectutil"
	"github.com/grafana/alloy/syntax/internal/syntaxtags"
	"github.com/grafana/alloy/syntax/internal/value"
	"github.com/grafana/alloy/syntax/token/builder"
)

var goAlloyDefaulter = reflect.TypeOf((*value.Defaulter)(nil)).Elem()

// MarshalBody marshals the provided Go value to a JSON representation of
// Alloy. MarshalBody panics if not given a struct with alloy tags or a
// map[string]any.
func MarshalBody(val interface{}) ([]byte, error) {
	rv := reflect.ValueOf(val)
	return json.Marshal(encodeStructAsBody(rv))
}

func encodeStructAsBody(rv reflect.Value) jsonBody {
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return jsonBody{}
		}
		rv = rv.Elem()
	}

	if rv.Kind() == reflect.Invalid {
		return jsonBody{}
	}

	body := jsonBody{}

	switch rv.Kind() {
	case reflect.Struct:
		fields := syntaxtags.Get(rv.Type())
		defaults := reflect.New(rv.Type()).Elem()
		if defaults.CanAddr() && defaults.Addr().Type().Implements(goAlloyDefaulter) {
			defaults.Addr().Interface().(value.Defaulter).SetToDefault()
		}

		for _, field := range fields {
			fieldVal := reflectutil.Get(rv, field)
			fieldValDefault := reflectutil.Get(defaults, field)

			isEqual := fieldVal.Comparable() && fieldVal.Equal(fieldValDefault)
			isZero := fieldValDefault.IsZero() && fieldVal.IsZero()

			if field.IsOptional() && (isEqual || isZero) {
				continue
			}

			body = append(body, encodeFieldAsStatements(nil, field, fieldVal)...)
		}

	case reflect.Map:
		if rv.Type().Key().Kind() != reflect.String {
			panic("syntax/encoding/alloyjson: unsupported map type; expected map[string]T, got " + rv.Type().String())
		}

		iter := rv.MapRange()
		for iter.Next() {
			mapKey, mapValue := iter.Key(), iter.Value()

			body = append(body, jsonAttr{
				Name:  mapKey.String(),
				Type:  "attr",
				Value: buildJSONValue(value.FromRaw(mapValue)),
			})
		}

	default:
		panic(fmt.Sprintf("syntax/encoding/alloyjson: can only encode struct or map[string]T values to bodies, got %s", rv.Kind()))
	}

	return body
}

// encodeFieldAsStatements encodes an individual field from a struct as a set
// of statements. One field may map to multiple statements in the case of a
// slice of blocks.
func encodeFieldAsStatements(prefix []string, field syntaxtags.Field, fieldValue reflect.Value) []jsonStatement {
	fieldName := strings.Join(field.Name, ".")

	for fieldValue.Kind() == reflect.Pointer {
		if fieldValue.IsNil() {
			break
		}
		fieldValue = fieldValue.Elem()
	}

	switch {
	case field.IsAttr():
		return []jsonStatement{jsonAttr{
			Name:  fieldName,
			Type:  "attr",
			Value: buildJSONValue(value.FromRaw(fieldValue)),
		}}

	case field.IsBlock():
		fullName := mergeStringSlice(prefix, field.Name)

		switch {
		case fieldValue.Kind() == reflect.Map:
			// Iterate over the map and add each element as an attribute into it.

			if fieldValue.Type().Key().Kind() != reflect.String {
				panic("syntax/encoding/alloyjson: unsupported map type for block; expected map[string]T, got " + fieldValue.Type().String())
			}

			statements := []jsonStatement{}

			iter := fieldValue.MapRange()
			for iter.Next() {
				mapKey, mapValue := iter.Key(), iter.Value()

				statements = append(statements, jsonAttr{
					Name:  mapKey.String(),
					Type:  "attr",
					Value: buildJSONValue(value.FromRaw(mapValue)),
				})
			}

			return []jsonStatement{jsonBlock{
				Name: strings.Join(fullName, "."),
				Type: "block",
				Body: statements,
			}}

		case fieldValue.Kind() == reflect.Slice, fieldValue.Kind() == reflect.Array:
			statements := []jsonStatement{}

			for i := 0; i < fieldValue.Len(); i++ {
				elem := fieldValue.Index(i)

				// Recursively call encodeField for each element in the slice/array.
				// The recursive call will hit the case below and add a new block for
				// each field encountered.
				statements = append(statements, encodeFieldAsStatements(prefix, field, elem)...)
			}

			return statements

		case fieldValue.Kind() == reflect.Struct:
			if fieldValue.IsZero() {
				// It shouldn't be possible to have a required block which is unset, but
				// we'll encode something anyway.
				return []jsonStatement{jsonBlock{
					Name: strings.Join(fullName, "."),
					Type: "block",

					// Never set this to nil, since the API contract always expects blocks
					// to have an array value for the body.
					Body: []jsonStatement{},
				}}
			}

			return []jsonStatement{jsonBlock{
				Name:  strings.Join(fullName, "."),
				Type:  "block",
				Label: getBlockLabel(fieldValue),
				Body:  encodeStructAsBody(fieldValue),
			}}
		}

	case field.IsEnum():
		// Blocks within an enum have a prefix set.
		newPrefix := mergeStringSlice(prefix, field.Name)

		switch {
		case fieldValue.Kind() == reflect.Slice, fieldValue.Kind() == reflect.Array:
			statements := []jsonStatement{}
			for i := 0; i < fieldValue.Len(); i++ {
				statements = append(statements, encodeEnumElementToStatements(newPrefix, fieldValue.Index(i))...)
			}
			return statements

		default:
			panic(fmt.Sprintf("syntax/encoding/alloyjson: unrecognized enum kind %s", fieldValue.Kind()))
		}
	}

	return nil
}

func mergeStringSlice(a, b []string) []string {
	if len(a) == 0 {
		return b
	} else if len(b) == 0 {
		return a
	}

	res := make([]string, 0, len(a)+len(b))
	res = append(res, a...)
	res = append(res, b...)
	return res
}

// getBlockLabel returns the label for a given block.
func getBlockLabel(rv reflect.Value) string {
	tags := syntaxtags.Get(rv.Type())
	for _, tag := range tags {
		if tag.Flags&syntaxtags.FlagLabel != 0 {
			return reflectutil.Get(rv, tag).String()
		}
	}

	return ""
}

func encodeEnumElementToStatements(prefix []string, enumElement reflect.Value) []jsonStatement {
	for enumElement.Kind() == reflect.Pointer {
		if enumElement.IsNil() {
			return nil
		}
		enumElement = enumElement.Elem()
	}

	fields := syntaxtags.Get(enumElement.Type())

	statements := []jsonStatement{}

	// Find the first non-zero field and encode it.
	for _, field := range fields {
		fieldVal := reflectutil.Get(enumElement, field)
		if !fieldVal.IsValid() || fieldVal.IsZero() {
			continue
		}

		statements = append(statements, encodeFieldAsStatements(prefix, field, fieldVal)...)
		break
	}

	return statements
}

// MarshalValue marshals the provided Go value to a JSON representation of
// Alloy.
func MarshalValue(val interface{}) ([]byte, error) {
	alloyValue := value.Encode(val)
	return json.Marshal(buildJSONValue(alloyValue))
}

func buildJSONValue(v value.Value) jsonValue {
	if tk, ok := v.Interface().(builder.Tokenizer); ok {
		return jsonValue{
			Type:  "capsule",
			Value: tk.AlloyTokenize()[0].Lit,
		}
	}

	switch v.Type() {
	case value.TypeNull:
		return jsonValue{Type: "null"}

	case value.TypeNumber:
		return jsonValue{Type: "number", Value: v.Number().Float()}

	case value.TypeString:
		return jsonValue{Type: "string", Value: v.Text()}

	case value.TypeBool:
		return jsonValue{Type: "bool", Value: v.Bool()}

	case value.TypeArray:
		elements := []interface{}{}

		for i := 0; i < v.Len(); i++ {
			element := v.Index(i)

			elements = append(elements, buildJSONValue(element))
		}

		return jsonValue{Type: "array", Value: elements}

	case value.TypeObject:
		return tokenizeObject(v)

	case value.TypeFunction:
		return jsonValue{Type: "function", Value: v.Describe()}

	case value.TypeCapsule:
		// Check if this capsule can be converted into Alloy object for more detailed description:
		if newVal, ok := v.TryConvertToObject(); ok {
			return tokenizeObject(value.Encode(newVal))
		}
		// Otherwise, describe the value
		return jsonValue{Type: "capsule", Value: v.Describe()}

	default:
		panic(fmt.Sprintf("syntax/encoding/alloyjson: unrecognized value type %q", v.Type()))
	}
}

func tokenizeObject(v value.Value) jsonValue {
	keys := v.Keys()

	// If v isn't an ordered object (i.e., a go map), sort the keys so they
	// have a deterministic print order.
	if !v.OrderedKeys() {
		sort.Strings(keys)
	}

	fields := []jsonObjectField{}

	for i := 0; i < len(keys); i++ {
		field, _ := v.Key(keys[i])

		fields = append(fields, jsonObjectField{
			Key:   keys[i],
			Value: buildJSONValue(field),
		})
	}

	return jsonValue{Type: "object", Value: fields}
}
