package adaptive

import "time"

// Policy — контракт політики адаптивної конкуренції.
//
// Next повертає:
// - next: наступне значення конкуренції (вже обмежене в допустимих межах політики),
// - reason: коротка причина рішення (для телеметрії/логування),
// - changed: чи відбулося реальне оновлення (next != cur).
type Policy interface {
	Next(cur int, sig AdaptiveSignal, now time.Time) (next int, reason string, changed bool)
}
