use std::{env, fs, path::PathBuf};

fn main() {
    let crate_dir = PathBuf::from(env::var("CARGO_MANIFEST_DIR").unwrap());
    let config_path = crate_dir.join("cbindgen.toml");
    let output_dir = crate_dir.join("include");
    fs::create_dir_all(&output_dir).expect("create generated header directory");

    cbindgen::generate_with_config(
        &crate_dir,
        cbindgen::Config::from_file(&config_path).expect("read cbindgen config"),
    )
    .expect("generate OPAQUE C header")
    .write_to_file(output_dir.join("arkive_opaque.h"));

    println!("cargo:rerun-if-changed=src");
    println!("cargo:rerun-if-changed=cbindgen.toml");
}
