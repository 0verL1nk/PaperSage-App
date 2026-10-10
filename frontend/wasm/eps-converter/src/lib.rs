use stet::Interpreter;
use wasm_bindgen::prelude::*;

#[wasm_bindgen]
extern "C" {
    #[wasm_bindgen(js_namespace = globalThis, js_name = __papersageStetPanic)]
    fn report_stet_panic(message: &str);
}

#[wasm_bindgen]
pub fn convert_eps_to_pdf(input: &[u8], creation_date: &str) -> Result<Vec<u8>, JsError> {
    std::panic::set_hook(Box::new(|info| {
        console_error_panic_hook::hook(info);
        report_stet_panic(&info.to_string());
    }));
    if input.is_empty() {
        return Err(JsError::new("EPS 文件为空"));
    }
    if input.len() > 32 * 1024 * 1024 {
        return Err(JsError::new("EPS 文件超过浏览器转换器的 32 MiB 限制"));
    }
    let date_bytes = creation_date.as_bytes();
    if date_bytes.len() != 17
        || &date_bytes[..2] != b"D:"
        || date_bytes[16] != b'Z'
        || !date_bytes[2..16].iter().all(u8::is_ascii_digit)
    {
        return Err(JsError::new("PDF 创建时间必须采用 D:YYYYMMDDHHmmssZ 格式"));
    }

    let mut interpreter = Interpreter::builder().suppress_output().build();
    let mut postscript = Vec::with_capacity(input.len() + 64);
    postscript.extend_from_slice(input);
    postscript.extend_from_slice(format!("\n[ /CreationDate ({creation_date}) /DOCINFO pdfmark\n").as_bytes());
    let pdf = interpreter
        .render_to_pdf(&postscript, 72.0)
        .map_err(|error| JsError::new(&error.to_string()))?;
    if pdf.is_empty() {
        return Err(JsError::new("PostScript 引擎没有生成 PDF 页面"));
    }
    Ok(pdf)
}
