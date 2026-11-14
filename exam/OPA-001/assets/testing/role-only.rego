package tenant

default allow := false

allow if input.role == "reader"
