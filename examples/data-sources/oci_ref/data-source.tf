# Terraform reads the tag once per run. If another push moves the tag between
# plan and apply, everything below still uses the digest Terraform read.
data "oci_ref" "base" {
  ref = "example.com/test:latest"
}

output "digest" {
  value = data.oci_ref.base.digest
}

output "full_ref" {
  value = data.oci_ref.base.full_ref
}

output "linux_amd64" {
  value = data.oci_ref.base.images["linux/amd64"].image_ref
}
