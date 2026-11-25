{
  nix2container,
  system,
  runCommand,
  cacert,
  backend,
  frontend,
  version,
  maintainers,
  license,
}:
let
  n2c = nix2container.packages.${system}.nix2container;

  # Create a /tmp directory for the container
  tmpDir = runCommand "tmp-dir" { } ''
    mkdir -p $out/tmp
  '';

  # Create a writable home directory for the nonroot user
  homeDir = runCommand "home-dir" { } ''
    mkdir -p $out/home/nonroot
  '';
in
n2c.buildImage {
  name = "ghcr.io/adrient-tech/pdf";
  tag = version;
  copyToRoot = [
    cacert
  ];
  layers = [
    (n2c.buildLayer {
      copyToRoot = [
        tmpDir
        homeDir
      ];
      perms = [
        {
          path = tmpDir;
          regex = "/tmp";
          mode = "1777";
          uid = 65532;
          gid = 65532;
          uname = "nonroot";
          gname = "nonroot";
        }
        {
          path = homeDir;
          regex = "/home/nonroot";
          uid = 65532;
          gid = 65532;
          uname = "nonroot";
          gname = "nonroot";
        }
      ];
      metadata = {
        created_by = "nix2container";
        author = "Adrient";
      };
    })
    (n2c.buildLayer {
      copyToRoot = [ frontend ];
      perms = [
        {
          path = frontend;
          uid = 65532;
          gid = 65532;
          uname = "nonroot";
          gname = "nonroot";
        }
      ];
      metadata = {
        created_by = "nix2container";
        author = "Adrient";
      };
    })
    (n2c.buildLayer {
      copyToRoot = [ backend ];
      perms = [
        {
          path = backend;
          uid = 65532;
          gid = 65532;
          uname = "nonroot";
          gname = "nonroot";
        }
      ];
      metadata = {
        created_by = "nix2container";
        author = "Adrient";
      };
    })
  ];
  config = {
    env = [
      "STATIC_DIR=/usr/share/html"
      "HOME=/home/nonroot"
    ];
    entrypoint = [
      "./usr/bin/pdf"
    ];
    user = "65532:65532";
  };

  meta = {
    description = "Pdf OCI container image with backend and frontend";
    homepage = "https://github.com/Adrient-tech/pdf";
    inherit license;
    maintainers = with maintainers; [
      alex
    ];
    platforms = [ "x86_64-linux" ];
  };
}
