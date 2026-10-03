package game

// DepositGoldToBank moves currency without changing the total or wrapping a balance.
func (c *Character) DepositGoldToBank(amount uint32) bool {
	if amount == 0 || amount > c.Gold || uint64(c.BankGold)+uint64(amount) > MaxBankGold {
		return false
	}
	c.Gold -= amount
	c.BankGold += amount
	return true
}

// WithdrawGoldFromBank refuses amounts that would exceed the carrying limit.
// Check capacity before debiting the bank; the reference can lose gold at this limit.
func (c *Character) WithdrawGoldFromBank(amount uint32) bool {
	if amount == 0 || amount > c.BankGold || uint64(c.Gold)+uint64(amount) > MaxGold {
		return false
	}
	c.BankGold -= amount
	c.Gold += amount
	return true
}
