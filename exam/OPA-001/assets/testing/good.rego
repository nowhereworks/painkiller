package tenant

default allow := false

allow if {
	input.role == "reader"
	input.tenant == input.resource_tenant
}
