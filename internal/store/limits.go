package store

const (
	UserIDOffset            = 10_000
	SecondCharacterIDOffset = 4_500_000
	UsernameMinBytes        = 4
	UsernameMaxBytes        = 14
	PasswordMinBytes        = 4
	PasswordMaxBytes        = 14
	DeletionCodeMinBytes    = 6
	DeletionCodeMaxBytes    = 14
	passwordIterations      = 120_000
	passwordSaltBytes       = 16
	passwordKeyBytes        = 32
	passwordHashParts       = 4
	passwordHashAlgorithm   = "pbkdf2-sha256"
	asciiPrintableMin       = 32
	asciiPrintableMax       = 126
	maxEmailBytes           = 254
	adminAccountListLimit   = 1000
	maxSettingBytes         = 1000
	maxServerNameBytes      = 200
)
