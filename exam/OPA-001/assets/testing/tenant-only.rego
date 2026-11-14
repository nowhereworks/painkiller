package tenant

default allow := false

allow if input.tenant == input.resource_tenant
