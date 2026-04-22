resource "google_iam_workload_identity_pool" "github" {
  project                   = var.project_id
  workload_identity_pool_id = "overseas-safety-map-pool"
  display_name              = "GitHub Actions pool"

  depends_on = [google_project_service.enabled]
}

resource "google_iam_workload_identity_pool_provider" "github" {
  project                            = var.project_id
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = "github-provider"
  display_name                       = "GitHub OIDC provider"

  attribute_mapping = {
    "google.subject"       = "assertion.sub"
    "attribute.repository" = "assertion.repository"
    "attribute.ref"        = "assertion.ref"
    "attribute.actor"      = "assertion.actor"
  }

  # Gate at the provider level: only identity tokens from the expected
  # repository can exchange. Per-branch scoping is enforced on the SA binding
  # below (principalSet keyed on attribute.ref).
  attribute_condition = "assertion.repository == '${var.github_repository}'"

  oidc {
    issuer_uri = "https://token.actions.githubusercontent.com"
  }
}

# Only identity tokens issued for a workflow running on the main branch
# (i.e. post-merge deploys) may impersonate the high-privilege ci-deployer
# service account. PR workflows cannot acquire credentials through this
# binding, so a compromised branch cannot invoke deploy.yml or apply
# Terraform changes. PR terraform-plan runs `fmt` / `validate` without
# cloud credentials; any user who needs a real plan should run it locally
# with ADC before opening the PR.
resource "google_service_account_iam_binding" "ci_deployer_wif" {
  service_account_id = google_service_account.ci_deployer.name
  role               = "roles/iam.workloadIdentityUser"

  members = [
    "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.ref/refs/heads/main",
  ]
}
