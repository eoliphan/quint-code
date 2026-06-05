//! haft-embed — local embedding sidecar.
//!
//! Speaks newline-delimited JSON over stdio so haft can spawn it as a child
//! process and stream embedding requests without a vector DB or a network hop.
//! Protocol:
//!   - on startup, emits one handshake line: {"ready":true,"model":..,"dim":N}
//!     (or {"ready":false,"error":..} + exit 1 if the model fails to load)
//!   - then, per input line: request  {"id":N,"task":"query|document|raw","texts":[..]}
//!                           response {"id":N,"vectors":[[f32..]..]}  | {"id":N,"error":..}
//!   - EOF on stdin -> exit 0.
//!
//! The sidecar is OPTIONAL: when absent, haft degrades to FTS5+PPR recall.
//! It exists only to AUGMENT that recall with semantic similarity — the
//! decision graph stays primary (dec-20260605-fe77b358).

use std::io::{self, BufRead, Write};
use std::path::PathBuf;

use anyhow::{anyhow, Context, Result};
use clap::Parser;
use fastembed::{EmbeddingModel, InitOptions, TextEmbedding};
use serde::{Deserialize, Serialize};

/// Local embedding sidecar for haft.
#[derive(Parser, Debug)]
#[command(name = "haft-embed", version, about)]
struct Args {
    /// Embedding model id (embeddinggemma-300m | bge-small-en-v1.5).
    #[arg(long, default_value = "embeddinggemma-300m")]
    model: String,

    /// Directory where the ONNX model is cached/downloaded (haft passes ~/.haft/models).
    #[arg(long)]
    cache_dir: Option<PathBuf>,

    /// Matryoshka (MRL) truncation target dimension (768/512/256/128). 0 = native.
    #[arg(long, default_value_t = 0)]
    dim: usize,

    /// Show model-download progress on stderr (kept off so stdout stays a clean JSON stream).
    #[arg(long)]
    show_progress: bool,
}

#[derive(Deserialize)]
struct Request {
    id: u64,
    #[serde(default)]
    task: Task,
    texts: Vec<String>,
}

/// Retrieval role. EmbeddingGemma is asymmetric: queries and documents get
/// distinct canonical prompt prefixes (Google's spec). `raw` opts out.
#[derive(Deserialize, Default, Clone, Copy, PartialEq)]
#[serde(rename_all = "lowercase")]
enum Task {
    #[default]
    Query,
    Document,
    Raw,
}

#[derive(Serialize)]
struct Handshake {
    ready: bool,
    model: String,
    dim: usize,
}

#[derive(Serialize)]
struct Response {
    id: u64,
    #[serde(skip_serializing_if = "Option::is_none")]
    vectors: Option<Vec<Vec<f32>>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    error: Option<String>,
}

// ---- functional core: pure transforms, no IO ----

fn resolve_model(name: &str) -> Result<EmbeddingModel> {
    match name.to_ascii_lowercase().replace('_', "-").as_str() {
        "embeddinggemma-300m" | "embeddinggemma" | "gemma" => Ok(EmbeddingModel::EmbeddingGemma300M),
        "bge-small-en-v1.5" | "bge-small" => Ok(EmbeddingModel::BGESmallENV15),
        other => Err(anyhow!("unknown model id: {other}")),
    }
}

/// Apply EmbeddingGemma's canonical asymmetric prompt prefix for the role.
fn prefix(task: Task, text: &str) -> String {
    match task {
        Task::Query => format!("task: search result | query: {text}"),
        Task::Document => format!("title: none | text: {text}"),
        Task::Raw => text.to_string(),
    }
}

/// MRL-truncate to `dim` (Matryoshka prefix dims are valid for EmbeddingGemma),
/// then L2-normalize so downstream cosine is a plain dot product.
fn truncate_normalize(mut vector: Vec<f32>, dim: usize) -> Vec<f32> {
    if dim > 0 && dim < vector.len() {
        vector.truncate(dim);
    }
    let norm: f32 = vector.iter().map(|value| value * value).sum::<f32>().sqrt();
    if norm > 0.0 {
        vector.iter_mut().for_each(|value| *value /= norm);
    }
    vector
}

// ---- effect boundary: the model handle ----

struct Embedder {
    model: TextEmbedding,
    dim: usize,
}

impl Embedder {
    fn load(args: &Args) -> Result<Self> {
        let kind = resolve_model(&args.model)?;
        let mut options = InitOptions::new(kind).with_show_download_progress(args.show_progress);
        if let Some(dir) = &args.cache_dir {
            options = options.with_cache_dir(dir.clone());
        }
        let mut model = TextEmbedding::try_new(options).context("load embedding model")?;

        let probe = model.embed(vec!["x"], None).context("probe embedding")?;
        let native = probe.first().map(Vec::len).unwrap_or(0);
        let dim = if args.dim > 0 && args.dim < native { args.dim } else { native };
        Ok(Self { model, dim })
    }

    fn embed(&mut self, task: Task, texts: &[String]) -> Result<Vec<Vec<f32>>> {
        let prepared: Vec<String> = texts.iter().map(|text| prefix(task, text)).collect();
        let vectors = self
            .model
            .embed(prepared, None)
            .context("embed texts")?
            .into_iter()
            .map(|vector| truncate_normalize(vector, self.dim))
            .collect();
        Ok(vectors)
    }
}

fn handle_line(embedder: &mut Embedder, line: &str) -> Response {
    let request: Request = match serde_json::from_str(line) {
        Ok(request) => request,
        Err(err) => {
            return Response { id: 0, vectors: None, error: Some(format!("bad request json: {err}")) }
        }
    };
    match embedder.embed(request.task, &request.texts) {
        Ok(vectors) => Response { id: request.id, vectors: Some(vectors), error: None },
        Err(err) => Response { id: request.id, vectors: None, error: Some(format!("{err:#}")) },
    }
}

// ---- imperative shell ----

fn main() -> Result<()> {
    let args = Args::parse();
    let stdout = io::stdout();
    let mut out = stdout.lock();

    let mut embedder = match Embedder::load(&args) {
        Ok(embedder) => embedder,
        Err(err) => {
            let line = serde_json::json!({ "ready": false, "error": format!("{err:#}") });
            writeln!(out, "{line}")?;
            out.flush()?;
            std::process::exit(1);
        }
    };

    let handshake = Handshake { ready: true, model: args.model.clone(), dim: embedder.dim };
    writeln!(out, "{}", serde_json::to_string(&handshake)?)?;
    out.flush()?;

    let stdin = io::stdin();
    for line in stdin.lock().lines() {
        let line = line?;
        if line.trim().is_empty() {
            continue;
        }
        let response = handle_line(&mut embedder, &line);
        writeln!(out, "{}", serde_json::to_string(&response)?)?;
        out.flush()?;
    }
    Ok(())
}
