package actions

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/rs/zerolog"

	"github.com/BoSuY0/tetracore-worker/internal/domain"
)

// mockAction — мок-реалізація domain.ActionHandler для тестування реєстру.
type mockAction struct {
	name   string
	result domain.ActionResult
	err    error
}

func (m *mockAction) Name() string { return m.name }
func (m *mockAction) Execute(_ context.Context, _ domain.ActionParams) (domain.ActionResult, error) {
	return m.result, m.err
}

// newTestRegistry створює реєстр з вимкненим логером для тестів.
func newTestRegistry() *Registry {
	return NewRegistry().WithLogger(zerolog.Nop())
}

// ---------------------------------------------------------------------------
// TestRegistry_RegisterAndGet — реєстрація обробника та отримання за назвою.
// ---------------------------------------------------------------------------
func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := newTestRegistry()

	handler := &mockAction{name: "test_action"}
	reg.Register(handler)

	got := reg.Get("test_action")
	if got == nil {
		t.Fatal("Get повернув nil для зареєстрованого обробника")
	}
	if got.Name() != "test_action" {
		t.Fatalf("очікувалось ім'я %q, отримано %q", "test_action", got.Name())
	}

	// Get для незареєстрованого — очікуємо nil.
	if reg.Get("nonexistent") != nil {
		t.Fatal("Get повинен повертати nil для незареєстрованого action")
	}
}

// ---------------------------------------------------------------------------
// TestRegistry_Has — перевірка наявності/відсутності обробника.
// ---------------------------------------------------------------------------
func TestRegistry_Has(t *testing.T) {
	reg := newTestRegistry()

	reg.Register(&mockAction{name: "exists"})

	if !reg.Has("exists") {
		t.Fatal("Has повинен повертати true для зареєстрованого обробника")
	}
	if reg.Has("does_not_exist") {
		t.Fatal("Has повинен повертати false для незареєстрованого обробника")
	}
}

// ---------------------------------------------------------------------------
// TestRegistry_Execute — виконання зареєстрованого обробника.
// ---------------------------------------------------------------------------
func TestRegistry_Execute(t *testing.T) {
	reg := newTestRegistry()

	expectedResult := domain.NewSuccessResult(map[string]any{"key": "value"})
	handler := &mockAction{
		name:   "do_work",
		result: expectedResult,
		err:    nil,
	}
	reg.Register(handler)

	params := domain.ActionParams{
		TaskID: "task-1",
		Action: "do_work",
	}

	result, err := reg.Execute(context.Background(), "do_work", params)
	if err != nil {
		t.Fatalf("Execute повернув неочікувану помилку: %v", err)
	}
	if !result.Success {
		t.Fatal("очікувався успішний результат, отримано Success=false")
	}
	if result.Data["key"] != "value" {
		t.Fatalf("очікувалось Data[\"key\"]=\"value\", отримано %v", result.Data["key"])
	}

	// Також перевіримо випадок, коли обробник повертає помилку.
	handlerErr := errors.New("handler failure")
	failHandler := &mockAction{
		name:   "fail_work",
		result: domain.ActionResult{},
		err:    handlerErr,
	}
	reg.Register(failHandler)

	_, err = reg.Execute(context.Background(), "fail_work", params)
	if !errors.Is(err, handlerErr) {
		t.Fatalf("очікувалась помилка обробника %q, отримано %v", handlerErr, err)
	}
}

// ---------------------------------------------------------------------------
// TestRegistry_ExecuteUnknown — виконання незареєстрованого action.
// ---------------------------------------------------------------------------
func TestRegistry_ExecuteUnknown(t *testing.T) {
	reg := newTestRegistry()

	result, err := reg.Execute(context.Background(), "ghost_action", domain.ActionParams{})
	if !errors.Is(err, domain.ErrUnknownAction) {
		t.Fatalf("очікувалась domain.ErrUnknownAction, отримано: %v", err)
	}
	if result.Success {
		t.Fatal("результат для невідомого action не повинен бути Success=true")
	}
	if result.Error == nil {
		t.Fatal("результат для невідомого action повинен містити Error")
	}
	if result.Error.Code != "UNKNOWN_ACTION" {
		t.Fatalf("очікувався код помилки %q, отримано %q", "UNKNOWN_ACTION", result.Error.Code)
	}
}

// ---------------------------------------------------------------------------
// TestRegistry_ListActions — перевірка відсортованого списку action-ів.
// ---------------------------------------------------------------------------
func TestRegistry_ListActions(t *testing.T) {
	reg := newTestRegistry()

	// Реєструємо в довільному порядку.
	names := []string{"zeta", "alpha", "mu", "beta"}
	for _, n := range names {
		reg.Register(&mockAction{name: n})
	}

	list := reg.ListActions()
	expected := []string{"alpha", "beta", "mu", "zeta"}

	if len(list) != len(expected) {
		t.Fatalf("очікувалось %d елементів, отримано %d", len(expected), len(list))
	}
	for i, name := range expected {
		if list[i] != name {
			t.Fatalf("позиція %d: очікувалось %q, отримано %q", i, name, list[i])
		}
	}

	// Порожній реєстр — порожній список.
	emptyReg := newTestRegistry()
	if len(emptyReg.ListActions()) != 0 {
		t.Fatal("ListActions для порожнього реєстру повинен повертати порожній список")
	}
}

// ---------------------------------------------------------------------------
// TestRegistry_Len — перевірка підрахунку зареєстрованих обробників.
// ---------------------------------------------------------------------------
func TestRegistry_Len(t *testing.T) {
	reg := newTestRegistry()

	if reg.Len() != 0 {
		t.Fatalf("порожній реєстр: очікувалось Len()=0, отримано %d", reg.Len())
	}

	reg.Register(&mockAction{name: "a"})
	reg.Register(&mockAction{name: "b"})
	reg.Register(&mockAction{name: "c"})

	if reg.Len() != 3 {
		t.Fatalf("після 3 реєстрацій: очікувалось Len()=3, отримано %d", reg.Len())
	}

	// Перезапис не збільшує лічильник.
	reg.Register(&mockAction{name: "a"})
	if reg.Len() != 3 {
		t.Fatalf("після перезапису: очікувалось Len()=3, отримано %d", reg.Len())
	}
}

// ---------------------------------------------------------------------------
// TestRegistry_Overwrite — перезапис обробника з однаковим ім'ям.
// ---------------------------------------------------------------------------
func TestRegistry_Overwrite(t *testing.T) {
	reg := newTestRegistry()

	first := &mockAction{
		name:   "dup",
		result: domain.NewSuccessResult(map[string]any{"version": 1}),
	}
	second := &mockAction{
		name:   "dup",
		result: domain.NewSuccessResult(map[string]any{"version": 2}),
	}

	reg.Register(first)
	reg.Register(second)

	// Перевіряємо, що Get повертає другий обробник.
	got := reg.Get("dup")
	if got == nil {
		t.Fatal("Get повернув nil після подвійної реєстрації")
	}

	result, err := got.Execute(context.Background(), domain.ActionParams{})
	if err != nil {
		t.Fatalf("неочікувана помилка: %v", err)
	}
	if result.Data["version"] != 2 {
		t.Fatalf("очікувалось version=2 (другий обробник), отримано %v", result.Data["version"])
	}

	// Execute через реєстр теж повинен використовувати другий обробник.
	result, err = reg.Execute(context.Background(), "dup", domain.ActionParams{})
	if err != nil {
		t.Fatalf("неочікувана помилка: %v", err)
	}
	if result.Data["version"] != 2 {
		t.Fatalf("Execute: очікувалось version=2, отримано %v", result.Data["version"])
	}
}

// ---------------------------------------------------------------------------
// TestRegistry_ConcurrentAccess — конкурентний доступ без data race.
// ---------------------------------------------------------------------------
func TestRegistry_ConcurrentAccess(t *testing.T) {
	reg := newTestRegistry()

	// Попередньо зареєструємо один обробник для читання.
	reg.Register(&mockAction{
		name:   "preloaded",
		result: domain.NewSuccessResult(map[string]any{"ok": true}),
	})

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()

			// Кожна горутина виконує мікс операцій.
			name := "preloaded"

			// Register — додаємо унікальний обробник для кожної горутини.
			reg.Register(&mockAction{
				name:   name,
				result: domain.NewSuccessResult(map[string]any{"idx": idx}),
			})

			// Has
			reg.Has(name)
			reg.Has("nonexistent")

			// Get
			reg.Get(name)

			// Execute зареєстрованого
			_, _ = reg.Execute(context.Background(), name, domain.ActionParams{})

			// Execute незареєстрованого
			_, _ = reg.Execute(context.Background(), "no_such_action", domain.ActionParams{})

			// ListActions
			reg.ListActions()

			// Len
			reg.Len()
		}(i)
	}

	wg.Wait()

	// Якщо дійшли сюди без паніки/deadlock — конкурентність працює коректно.
	if !reg.Has("preloaded") {
		t.Fatal("обробник 'preloaded' повинен існувати після конкурентного доступу")
	}
}
