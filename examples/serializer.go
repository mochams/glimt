package main

// CreateUserSerializer is a struct which represents the expected JSON body for a request to create a new user.
type CreateUserSerializer struct {
	Name   string `json:"name"`
	Email  string `json:"email"`
	Age    int    `json:"age"`
	Status string `json:"status"`
	Validator
}

// Validate executes the validation checks for the CreateUserSerializer fields.
func (s *CreateUserSerializer) Validate() {
	// Because the Validator struct is embedded by the CreateUserSerializer struct,
	// we can call CleanField() directly on it to execute our validation checks.
	s.CleanField(NotBlank(s.Name), "name", "This field cannot be blank")
	s.CleanField(NotBlank(s.Email), "email", "This field cannot be blank")
	s.CleanField(MaxChars(s.Name, 100), "name", "This field cannot be more than 100 characters long")
	s.CleanField(PermittedValue(s.Status, "active", "inactive"), "status", "This field must equal 'active' or 'inactive'")
}

// ToModel converts the CreateUserSerializer to a User model.
func (s *CreateUserSerializer) ToModel() User {
	return User{
		Name:   s.Name,
		Email:  s.Email,
		Age:    s.Age,
		Status: s.Status,
	}
}
