package misery

import (
	"errors"
	"fmt"
	"reflect"

	"github.com/iancoleman/strcase"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/yuin/stagparser"
)

var (
	ErrStructPointerRequired = errors.New("structure pointer required")
	ErrAttributeMalformed    = errors.New("attribute malformed")
	ErrTypeNotSupported      = errors.New("type not supported")
)

func RegisterMetrics(mtrcs interface{}, registry *prometheus.Registry) error {
	val, err := unpackStruct(mtrcs)
	if err != nil {
		return fmt.Errorf("struct unpack error: %w", err)
	}

	tags, err := parseStructTags(val)
	if err != nil {
		return fmt.Errorf("struct tag parse error: %w", err)
	}

	return registerMetricsByTags(val, tags, registry)
}

func unpackStruct(in interface{}) (val reflect.Value, err error) {
	ptrVal := reflect.ValueOf(in)
	if ptrVal.Kind() != reflect.Ptr {
		return val, ErrStructPointerRequired
	}

	val = ptrVal.Elem()
	if val.Kind() != reflect.Struct {
		return val, ErrStructPointerRequired
	}

	return val, nil
}

func parseStructTags(structValue reflect.Value) (map[string][]stagparser.Definition, error) {
	defs, err := stagparser.ParseStruct(structValue.Interface(), "misery")
	if err != nil {
		return nil, fmt.Errorf("tag parse error: %w", err)
	}

	tagMap := make(map[string][]stagparser.Definition, len(defs))
	for fieldName, fieldDefs := range defs {
		tagMap[fieldName] = fieldDefs
	}

	return tagMap, nil
}

var (
	prometheusCounterType   = reflect.TypeOf((*prometheus.CounterVec)(nil))
	prometheusHistogramType = reflect.TypeOf((*prometheus.HistogramVec)(nil))
)

func registerMetricsByTags(
	structValue reflect.Value,
	tags map[string][]stagparser.Definition,
	registry *prometheus.Registry,
) (err error) {
	for i := 0; i < structValue.NumField(); i++ {
		field := structValue.Field(i)
		typeField := structValue.Type().Field(i)
		var collector prometheus.Collector

		switch {
		case field.Type() == prometheusCounterType:
			if collector, err = createPrometheusCounter(typeField.Name, tags[typeField.Name]); err != nil {
				return fmt.Errorf("createPrometheusCounter failed: %w", err)
			}
		case field.Type() == prometheusHistogramType:
			if collector, err = createPrometheusHistogram(typeField.Name, tags[typeField.Name]); err != nil {
				return fmt.Errorf("createPrometheusHistogram failed: %w", err)
			}
		default:
			// return fmt.Errorf("%w: %v", ErrTypeNotSupported, field.Type())
			continue
		}

		field.Set(reflect.ValueOf(collector))
		if err := registry.Register(collector); err != nil {
			return fmt.Errorf("collector register failed for %s: %w", typeField.Name, err)
		}
	}

	return nil
}

func createPrometheusCounter(
	structFieldName string,
	defs []stagparser.Definition,
) (*prometheus.CounterVec, error) {
	name := strcase.ToSnake(structFieldName)
	labels := []string{}
	help := ""
	for _, def := range defs {
		attrs := def.Attributes()
		switch attrName := def.Name(); attrName {
		case "name":
			if nameString, ok := attrs[attrName].(string); ok {
				name = nameString
			} else {
				return nil, fmt.Errorf("%w: name is not a string", ErrAttributeMalformed)
			}
		case "labels":
			if labelSliceOfAny, ok := attrs[attrName].([]interface{}); ok {
				for _, labelInterface := range labelSliceOfAny {
					if labelString, ok := labelInterface.(string); ok {
						labels = append(labels, labelString)
					} else {
						return nil, fmt.Errorf("%w: label is not a string", ErrAttributeMalformed)
					}
				}
			} else {
				return nil, fmt.Errorf("%w: labels is not a list", ErrAttributeMalformed)
			}
		case "help":
			if helpString, ok := attrs[attrName].(string); ok {
				help = helpString
			} else {
				return nil, fmt.Errorf("%w: help is not a string", ErrAttributeMalformed)
			}
		default:
			return nil, fmt.Errorf("%w: unsupported attribute %s", ErrAttributeMalformed, attrName)
		}
	}

	return prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels), nil
}

func createPrometheusHistogram(
	structFieldName string,
	defs []stagparser.Definition,
) (*prometheus.HistogramVec, error) {
	opt := prometheus.HistogramOpts{
		Name:    strcase.ToSnake(structFieldName),
		Help:    "",
		Buckets: []float64{0.001, 0.01, 0.05, 0.1, 0.2, 0.3, 0.5, 1.0, 2.0, 10, 20},
	}
	labels := []string{}
	for _, def := range defs {
		attrs := def.Attributes()
		switch attrName := def.Name(); attrName {
		case "name":
			if nameString, ok := attrs[attrName].(string); ok {
				opt.Name = nameString
			} else {
				return nil, fmt.Errorf("%w: name is not a string", ErrAttributeMalformed)
			}
		case "labels":
			if labelSliceOfAny, ok := attrs[attrName].([]interface{}); ok {
				for _, labelInterface := range labelSliceOfAny {
					if labelString, ok := labelInterface.(string); ok {
						labels = append(labels, labelString)
					} else {
						return nil, fmt.Errorf("%w: label is not a string", ErrAttributeMalformed)
					}
				}
			} else {
				return nil, fmt.Errorf("%w: labels is not a list", ErrAttributeMalformed)
			}
		case "help":
			if helpString, ok := attrs[attrName].(string); ok {
				opt.Help = helpString
			} else {
				return nil, fmt.Errorf("%w: help is not a string", ErrAttributeMalformed)
			}
		case "buckets":
			if bucketSliceOfFAny, ok := attrs[attrName].([]interface{}); ok {
				opt.Buckets = make([]float64, 0, len(bucketSliceOfFAny))
				for _, bucketInterface := range bucketSliceOfFAny {
					switch b := bucketInterface.(type) {
					case float64:
						opt.Buckets = append(opt.Buckets, b)
					case int32:
						opt.Buckets = append(opt.Buckets, float64(b))
					case int64:
						opt.Buckets = append(opt.Buckets, float64(b))
					default:
						return nil, fmt.Errorf("%w: bucket is not a float64 %T %v", ErrAttributeMalformed, b, b)
					}
				}
			} else {
				return nil, fmt.Errorf("%w: buckets is not a list of floats", ErrAttributeMalformed)
			}
		default:
			return nil, fmt.Errorf("%w: unsupported attribute %s", ErrAttributeMalformed, attrName)
		}
	}

	return prometheus.NewHistogramVec(opt, labels), nil
}
