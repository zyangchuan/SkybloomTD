import { useState, useRef, useEffect } from 'react';
import Modal from './Modal';
import ButtonGreen from './ButtonGreen';
import ButtonWhite from './ButtonWhite';
import { getSupabaseBrowserClient } from '@/lib/supabase';

interface UploadDocumentModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export default function UploadDocumentModal({ isOpen, onClose }: UploadDocumentModalProps) {
  const [dragActive, setDragActive] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [isUploading, setIsUploading] = useState(false);
  const [uploadSuccess, setUploadSuccess] = useState(false);
  const [taskId, setTaskId] = useState<string | null>(null);
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Prevent browser default redirect behavior when dragging files onto the window
  useEffect(() => {
    if (!isOpen) return;

    const preventDefault = (e: DragEvent) => {
      e.preventDefault();
      e.stopPropagation();
    };

    window.addEventListener("dragover", preventDefault);
    window.addEventListener("drop", preventDefault);

    return () => {
      window.removeEventListener("dragover", preventDefault);
      window.removeEventListener("drop", preventDefault);
    };
  }, [isOpen]);

  const handleDrag = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    if (e.type === "dragenter" || e.type === "dragover") {
      setDragActive(true);
    } else if (e.type === "dragleave") {
      setDragActive(false);
    }
  };

  const handleDrop = (e: React.DragEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setDragActive(false);
    
    if (e.dataTransfer.files) {
      if (e.dataTransfer.files.length > 1) {
        setError("Only 1 PDF document can be uploaded at a time.");
        setFile(null);
        return;
      }
      if (e.dataTransfer.files[0]) {
        validateAndSetFile(e.dataTransfer.files[0]);
      }
    }
  };

  const handleChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    e.preventDefault();
    if (e.target.files && e.target.files[0]) {
      validateAndSetFile(e.target.files[0]);
    }
  };

  const validateAndSetFile = (selectedFile: File) => {
    setError(null);
    setUploadSuccess(false);
    setTaskId(null);

    // Verify it is a PDF
    if (selectedFile.type !== "application/pdf" && !selectedFile.name.endsWith(".pdf")) {
      setError("Only PDF documents are supported for OCR workflows.");
      setFile(null);
      return;
    }

    // Limit size to 10MB just as a healthy precaution
    if (selectedFile.size > 10 * 1024 * 1024) {
      setError("File exceeds 10MB limit.");
      setFile(null);
      return;
    }

    setFile(selectedFile);
  };

  const triggerFileSelect = () => {
    if (fileInputRef.current) {
      fileInputRef.current.click();
    }
  };

  const handleUpload = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!file) return;

    setIsUploading(true);
    setError(null);

    try {
      const supabase = getSupabaseBrowserClient();
      const { data: { session } } = await supabase.auth.getSession();
      const token = session?.access_token;

      if (!token) {
        throw new Error("Authentication token is missing. Please sign in again.");
      }

      const formData = new FormData();
      formData.append("file", file);

      // Call API shortcut route
      const res = await fetch("/api/upload-file", {
        method: "POST",
        headers: {
          "Authorization": `Bearer ${token}`
        },
        body: formData
      });

      if (!res.ok) {
        if (res.status === 413) {
          throw new Error("File exceeds server payload limits.");
        }
        const errJson = await res.json().catch(() => ({}));
        throw new Error(errJson.error || `Upload failed with status code ${res.status}`);
      }

      const data = await res.json();
      setTaskId(data.task_id || data.document_id);
      setUploadSuccess(true);
      setFile(null);
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : "An unexpected error occurred during upload.");
    } finally {
      setIsUploading(false);
    }
  };

  const formatFileSize = (bytes: number) => {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Upload Document">
      <div className="flex flex-col w-full gap-4 text-[#4a1900] select-none font-sans py-2">
        
        {uploadSuccess ? (
          <div className="flex flex-col items-center justify-center gap-4 py-6 text-center">
            <div className="text-2xl text-emerald-700 font-bold drop-shadow-[0_1px_0_rgba(255,255,255,0.4)]">
              🎉 Document Queued!
            </div>
            <p className="text-[#4a1900] text-md max-w-sm leading-relaxed font-semibold text-center">
              Your document has been securely uploaded. The OCR and indexing worker flow has been successfully initiated!
            </p>
            {taskId && (
              <div className="bg-[#4a1900]/5 border border-[#4a1900]/20 rounded p-2 text-xs font-mono text-[#4a1900]/80 select-text max-w-sm break-all text-center">
                Task ID: {taskId}
              </div>
            )}
            <ButtonGreen 
              className="h-10 w-44 text-lg font-bold text-white mt-2 drop-shadow-[0_1.5px_0_rgba(0,0,0,0.3)]"
              onClick={() => {
                setUploadSuccess(false);
                setTaskId(null);
              }}
            >
              Upload Another
            </ButtonGreen>
          </div>
        ) : (
          <form onSubmit={handleUpload} className="flex flex-col w-full gap-4">
            
            {/* Drag and Drop Zone */}
            <div
              onDragEnter={handleDrag}
              onDragOver={handleDrag}
              onDragLeave={handleDrag}
              onDrop={handleDrop}
              onClick={triggerFileSelect}
              className={`flex flex-col items-center justify-center w-full h-44 rounded-xl border-4 border-dashed cursor-pointer transition-all ${
                dragActive
                  ? "border-yellow-500 bg-yellow-500/10 scale-[1.01]"
                  : "border-[#4a1900]/30 hover:border-[#4a1900]/60 hover:bg-[#4a1900]/5 bg-transparent"
              }`}
            >
              <input
                ref={fileInputRef}
                type="file"
                className="hidden"
                accept=".pdf"
                multiple={false}
                onChange={handleChange}
                disabled={isUploading}
              />
              
              <div className="flex flex-col items-center gap-2 p-4 text-center">
                <span className="text-4xl animate-bounce">📄</span>
                <p className="font-bold text-lg drop-shadow-[0_0.5px_0_rgba(255,255,255,0.4)]">
                  {dragActive ? "Drop the PDF here!" : "Drag & Drop PDF here"}
                </p>
                <p className="text-xs text-[#4a1900]/70 font-semibold">
                  or click to browse from system
                </p>
              </div>
            </div>

            {/* Selected File Details */}
            {file && (
              <div className="flex items-center justify-between bg-yellow-500/10 border-2 border-yellow-500/30 rounded-xl p-3 w-full animate-fadeIn">
                <div className="flex flex-col items-start gap-0.5 max-w-[75%]">
                  <span className="font-bold text-sm truncate w-full text-[#4a1900]">
                    {file.name}
                  </span>
                  <span className="text-xs font-semibold text-[#4a1900]/65">
                    {formatFileSize(file.size)}
                  </span>
                </div>
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    setFile(null);
                  }}
                  className="text-red-700 hover:text-red-900 font-bold text-sm cursor-pointer p-1"
                  disabled={isUploading}
                >
                  Clear
                </button>
              </div>
            )}

            {/* Action CTAs */}
            <div className="flex gap-3 w-full mt-2">
              <ButtonWhite
                className="flex-1 h-11 text-lg font-bold"
                onClick={onClose}
                disabled={isUploading}
              >
                Cancel
              </ButtonWhite>
              
              <ButtonGreen
                type="submit"
                className={`flex-1 h-11 text-lg font-bold text-white drop-shadow-[0_1.5px_0_rgba(0,0,0,0.3)] ${isUploading ? 'disabled:cursor-wait' : ''}`}
                disabled={isUploading || !file}
              >
                {isUploading ? "Uploading..." : "Upload PDF"}
              </ButtonGreen>
            </div>

            {/* Error Message */}
            {error && (
              <div className="max-w-full text-xs font-mono text-red-900 bg-red-100/90 border border-red-400 rounded-lg py-2 px-3 mt-1 text-center select-text">
                {error}
              </div>
            )}
          </form>
        )}
      </div>
    </Modal>
  );
}
