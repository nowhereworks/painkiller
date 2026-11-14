package tenant

allow if {
	input.role == "reader"
	input.tenant == input.resource_tenant
}
