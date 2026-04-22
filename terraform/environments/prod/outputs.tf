output "artifact_registry_url" {
  value = module.shared.artifact_registry_url
}

output "workload_identity_provider" {
  value = module.shared.workload_identity_provider
}

output "ci_deployer_email" {
  value = module.shared.ci_deployer_email
}

output "bff_service_url" {
  value = module.bff.service_url
}

output "notifier_service_url" {
  value = module.notifier.service_url
}
