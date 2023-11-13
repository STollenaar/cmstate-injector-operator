exit_after_auth = ${exit_after_auth}
pid_file        = "/home/vault/.pid"

auto_auth = {
  method = {
    config = {
      role = "${internal_role_name}"
    }
    type = "kubernetes"
  }
  sink = {
    config = {
      path = "/vault/secrets/.token"
    }
    type = "file"
  }
}

template = {
  contents         = <<EOF
        {{- with secret "aws/creds/${aws_role_name}" -}}
        [default]
        aws_access_key_id = {{ .Data.access_key }}
        aws_secret_access_key = {{ .Data.secret_key }}
        aws_session_token = {{ .Data.security_token }}
        region = ca-central-1
        output = json
        {{- end }}
        EOF
  destination      = "/vault/secrets/aws/credentials"
  create_dest_dirs = true
}

vault = {
  address = "https://vault.vault.svc.cluster.local:8200"
}
