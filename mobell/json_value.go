package mobell

type jsonValue struct {
	v interface{}
}

func (v jsonValue) isNil() bool {
	return v.v == nil
}

func (v jsonValue) asMap() map[string]interface{} {
	if v.v == nil {
		return nil
	}

	m, _ := v.v.(map[string]interface{})

	return m
}

func (v jsonValue) asArr() []interface{} {
	if v.v == nil {
		return nil
	}

	a, _ := v.v.([]interface{})

	return a
}

func (v jsonValue) asInt() int {
	if v.v == nil {
		return 0
	}

	i, _ := v.v.(float64)

	return int(i)
}

func (v jsonValue) asString() string {
	if v.v == nil {
		return ""
	}

	s, _ := v.v.(string)

	return s
}

func (v jsonValue) asBool() bool {
	if v.v == nil {
		return false
	}

	b, ok := v.v.(bool)

	if !ok {
		b = "true" == v.asString()
	}

	return b
}

func (v jsonValue) mapGet(key string) jsonValue {
	m := v.asMap()

	if m == nil {
		return jsonValue{v: nil}
	}

	return jsonValue{v: m[key]}
}

func (v jsonValue) arrGet(pos int) jsonValue {
	a := v.asArr()

	if a == nil || pos < 0 || pos >= len(a) {
		return jsonValue{v: nil}
	}

	return jsonValue{v: a[pos]}
}
