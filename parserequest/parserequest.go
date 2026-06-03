package parserequest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
)

// If request Content-Type indicates JSON, parses as JSON.
//
// Otherwise, parses FormData to T. Only public properties are accessible.
//
// Uses either `form` struct tag, `json` struct tag, or lowercase property name as key.
//
// Pointers indicate a field is optional (can be assigned nil).
//
// Slices indicate that multiple values are acceptable.
func As[T any](r *http.Request) (T, error) {
	if isJSON(r) {
		return jsonDataAs[T](r)
	}

	return formDataAs[T](r)
}

func isJSON(r *http.Request) bool {
	contentType := r.Header.Get("Content-Type")

	return strings.Contains(contentType, "application/json")
}

func jsonDataAs[T any](r *http.Request) (T, error) {
	var zero T

	data, err := io.ReadAll(r.Body)

	if err != nil {
		return zero, err
	}

	var out T

	err = json.Unmarshal(data, &out)

	if err != nil {
		return zero, err
	}

	return out, nil
}

func formDataAs[T any](r *http.Request) (T, error) {
	var out T

	v := reflect.ValueOf(&out).Elem()
	t := v.Type()

	if t.Kind() != reflect.Struct {
		return out, fmt.Errorf("%T must be a struct", out)
	}

	err := r.ParseForm()

	if err != nil {
		return out, err
	}

	var formData map[string][]string = r.Form
	parseErrors := []error{}

	for i := range t.NumField() {
		field := t.Field(i)
		fieldValue := v.Field(i)
		err := setFromData(field, fieldValue, formData)

		if err != nil {
			parseErrors = append(parseErrors, err)
		}
	}

	if len(parseErrors) > 0 {
		return out, errors.Join(parseErrors...)
	}

	return out, nil
}

func nonEmpty(values []string) []string {
	valid := []string{}

	for _, value := range values {
		if value != "" {
			valid = append(valid, value)
		}
	}

	return valid
}

func setFromData(field reflect.StructField, fieldValue reflect.Value, formData map[string][]string) error {
	if !fieldValue.CanSet() {
		return nil
	}

	formKey := getFormKey(field)
	formValues := formData[formKey]
	formValues = nonEmpty(formValues)

	if field.Type.Kind() == reflect.Slice {
		if len(formValues) < 1 {
			return nil // Don't assign empty slice, just leave nil
		}

		slice := reflect.MakeSlice(field.Type, 0, len(formValues))

		for _, formValue := range formValues {
			elem, err := parseValue(formValue, field.Type.Elem())

			if err != nil {
				return fmt.Errorf("%s: %w", field.Name, err)
			}

			slice = reflect.Append(slice, elem)
		}

		fieldValue.Set(slice)
		return nil
	}

	if len(formValues) == 0 {
		// nil allowed
		if field.Type.Kind() == reflect.Pointer {
			return nil
		}

		return fmt.Errorf("missing value for %q", formKey)
	}

	if len(formValues) != 1 {
		return fmt.Errorf("%v values sent for %q, only one value permitted", len(formValues), formKey)
	}

	formValue := formValues[0]

	elem, err := parseValue(formValue, field.Type)

	if err != nil {
		return fmt.Errorf("%s: %w", field.Name, err)
	}

	fieldValue.Set(elem)

	return nil
}

func getFormKey(field reflect.StructField) string {
	key := field.Tag.Get("form")

	if key != "" {
		return key
	}

	key = field.Tag.Get("json")

	if key != "" {
		key, _, _ = strings.Cut(key, ",") // Removes modifiers

		return key
	}

	key = strings.ToLower(field.Name)

	return key
}

func parseValue(s string, t reflect.Type) (reflect.Value, error) {
	zero := reflect.Value{}

	switch t.Kind() {
	case reflect.String:
		return reflect.ValueOf(s).Convert(t), nil

	case reflect.Bool:
		v, err := strconv.ParseBool(s)

		if err != nil {
			return zero, err
		}

		return reflect.ValueOf(v).Convert(t), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v, err := strconv.ParseInt(s, 10, t.Bits())

		if err != nil {
			return zero, err
		}

		return reflect.ValueOf(v).Convert(t), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v, err := strconv.ParseUint(s, 10, t.Bits())

		if err != nil {
			return zero, err
		}

		return reflect.ValueOf(v).Convert(t), nil

	case reflect.Float32, reflect.Float64:
		v, err := strconv.ParseFloat(s, t.Bits())

		if err != nil {
			return zero, err
		}

		return reflect.ValueOf(v).Convert(t), nil

	case reflect.Pointer:
		realType := t.Elem()
		realValue, err := parseValue(s, realType)

		if err != nil {
			return zero, err
		}

		ptrValue := reflect.New(realType)
		ptrValue.Elem().Set(realValue)
		return ptrValue, nil
	}

	return zero, fmt.Errorf("unsupported type %s", t)
}
