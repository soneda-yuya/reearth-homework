locals {
  # IDs are declared once here so the pool/provider definitions, the audience
  # URL, and any principalSet strings that need them stay in sync.
  wif_pool_id     = "overseas-safety-map-pool"
  wif_provider_id = "github-provider"

  # The audience google-github-actions/auth@v2 sends by default: it matches
  # the full provider resource name under https://iam.googleapis.com/. Keeping
  # this in a local avoids hard-coding the IDs in two places.
  wif_audience = "https://iam.googleapis.com/projects/${var.project_number}/locations/global/workloadIdentityPools/${local.wif_pool_id}/providers/${local.wif_provider_id}"
}

resource "google_iam_workload_identity_pool" "github" {
  project                   = var.project_id
  workload_identity_pool_id = local.wif_pool_id
  display_name              = "GitHub Actions pool"

  depends_on = [google_project_service.enabled]
}

resource "google_iam_workload_identity_pool_provider" "github" {
  project                            = var.project_id
  workload_identity_pool_id          = google_iam_workload_identity_pool.github.workload_identity_pool_id
  workload_identity_pool_provider_id = local.wif_provider_id
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
    # Restrict the audience so a token minted for an unrelated GCP provider
    # cannot be replayed here. google-github-actions/auth@v2 sets this
    # audience by default when the workload_identity_provider input is
    # supplied, so no workflow change is needed.
    allowed_audiences = [local.wif_audience]
  }
}

# Only identity tokens issued for a workflow running on the main branch
# (i.e. post-merge deploys) may impersonate the high-privilege ci-deployer
# service account. PR workflows cannot acquire credentials through this
# binding, so a compromised branch cannot invoke deploy.yml or apply
# Terraform changes. PR terraform-plan runs `fmt` / `validate` without
# cloud credentials; a real plan is expected to be run locally with ADC.
#
# iam_member (not iam_binding) so other manually added members at the same
# role are preserved — e.g. an emergency break-glass binding made outside
# Terraform will not be silently erased on the next apply.
resource "google_service_account_iam_member" "ci_deployer_wif" {
  service_account_id = google_service_account.ci_deployer.name
  role               = "roles/iam.workloadIdentityUser"
  member             = "principalSet://iam.googleapis.com/${google_iam_workload_identity_pool.github.name}/attribute.ref/refs/heads/main"
}
