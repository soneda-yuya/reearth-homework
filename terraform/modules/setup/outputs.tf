output "runtime_sa_email" {
  value = google_service_account.runtime.email
}

output "job_name" {
  value = google_cloud_run_v2_job.setup.name
}
