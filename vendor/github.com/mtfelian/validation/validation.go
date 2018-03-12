package validation

import (
	"fmt"
	"regexp"
)

// ValidationError хранит сообщение об ошибке валидации
type ValidationError struct {
	Message string
}

// String возвращает поле Message из структуры ValidationError
func (e *ValidationError) String() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// Validation хранит сообщения об ошибках валидации
type Validation struct {
	Errors []*ValidationError
}

// String возвращает список ошибок валидации через переносы строк
func (v Validation) String() string {
	s := ""
	for _, message := range v.Errors {
		s += fmt.Sprintf("%s\n", message.String())
	}
	return s
}

// Clear удаляет все ошибки валидации
func (v *Validation) Clear() {
	v.Errors = []*ValidationError{}
}

// HasErrors возвращает true если есть хотя бы одна ошибка, иначе false
func (v *Validation) HasErrors() bool {
	return len(v.Errors) > 0
}

// Error добавляет ошибку в контекст валидации
func (v *Validation) Error(message string, args ...interface{}) *ValidationResult {
	result := (&ValidationResult{
		Ok:    false,
		Error: &ValidationError{},
	}).Message(message, args...)
	v.Errors = append(v.Errors, result.Error)
	return result
}

// ValidationResult возвращается для каждого способа валидации.
// Он предоставляет указание на успех и указатель на ошибку (если есть)
type ValidationResult struct {
	Error *ValidationError
	Ok    bool
}

// Message устанавливает сообщение об ошибке в ValidationResult,
// Возвращает себя. Допускает вызов по типу sprintf() со множеством параметров
func (r *ValidationResult) Message(message string, args ...interface{}) *ValidationResult {
	if r.Error != nil {
		if len(args) == 0 {
			r.Error.Message = message
		} else {
			r.Error.Message = fmt.Sprintf(message, args...)
		}
	}
	return r
}

// Required проверяет что аргумент не является nil и не является пустым (если это строка или срез)
func (v *Validation) Required(obj interface{}) *ValidationResult {
	return v.apply(Required{}, obj)
}

// Min проверяет что n >= min
func (v *Validation) Min(n int, min int) *ValidationResult {
	return v.apply(Min{min}, n)
}

// Max проверяет что n <= max
func (v *Validation) Max(n int, max int) *ValidationResult {
	return v.apply(Max{max}, n)
}

// Range проверяет что min <= n <= max
func (v *Validation) Range(n, min, max int) *ValidationResult {
	return v.apply(Range{Min{min}, Max{max}}, n)
}

// MinSize проверяет что размер объекта obj >= min
func (v *Validation) MinSize(obj interface{}, min int) *ValidationResult {
	return v.apply(MinSize{min}, obj)
}

// MaxSize проверяет что размер объекта obj <= max
func (v *Validation) MaxSize(obj interface{}, max int) *ValidationResult {
	return v.apply(MaxSize{max}, obj)
}

// Length проверяет что длина объекта obj равна n
func (v *Validation) Length(obj interface{}, n int) *ValidationResult {
	return v.apply(Length{n}, obj)
}

// Match проверяет что строка str соответствует регулярному выражению regex
func (v *Validation) Match(str string, regex *regexp.Regexp) *ValidationResult {
	return v.apply(Match{regex}, str)
}

// Email проверяет на корректность адрес e-mail в параметре str
func (v *Validation) Email(str string) *ValidationResult {
	return v.apply(Email{Match{emailPattern}}, str)
}

// apply применяет объект, реализующий интерфейс Validator к obj
// и возвращает результат валидации
func (v *Validation) apply(chk Validator, obj interface{}) *ValidationResult {
	if chk.IsSatisfied(obj) {
		return &ValidationResult{Ok: true}
	}

	// Добавляем ошибку в контекст валидации
	err := &ValidationError{
		Message: chk.DefaultMessage(),
	}
	v.Errors = append(v.Errors, err)

	// Так же возвращаем её
	return &ValidationResult{
		Ok:    false,
		Error: err,
	}
}

// Check применяет набор объектов checks, реализующих интерфейс Validator к полю obj,
// и возвращает ValidationResult от первой ошибки валидации, или от последней успешной.
func (v *Validation) Check(obj interface{}, checks ...Validator) *ValidationResult {
	var result *ValidationResult
	for _, check := range checks {
		result = v.apply(check, obj)
		if !result.Ok {
			return result
		}
	}
	return result
}
