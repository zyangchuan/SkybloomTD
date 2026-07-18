import { useState, useRef, useEffect } from 'react';
import { Sparkles, FileText, Info } from 'lucide-react';
import axios from 'axios';
import Modal from './Modal';
import ButtonGreen from './ButtonGreen';
import ButtonWhite from './ButtonWhite';
import RetroInput from './RetroInput';
import { getSupabaseBrowserClient } from '@/lib/supabase';
import { syncAuthCookie } from '@/lib/auth-cookie';

interface UploadDocumentModalProps {
  isOpen: boolean;
  onClose: () => void;
  onUploadSuccess?: () => void;
}

export default function UploadDocumentModal({ isOpen, onClose, onUploadSuccess }: UploadDocumentModalProps) {
  const [dragActive, setDragActive] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [gameName, setGameName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [isUploading, setIsUploading] = useState(false);
  const [uploadSuccess, setUploadSuccess] = useState(false);
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

  useEffect(() => {
    if (!isOpen) {
      setTimeout(() => {
        setFile(null);
        setGameName("");
        setError(null);
        setUploadSuccess(false);
      }, 0);
    }
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
    setGameName((current) => current.trim() || defaultGameNameFromFile(selectedFile));
  };

  const triggerFileSelect = () => {
    if (fileInputRef.current) {
      fileInputRef.current.click();
    }
  };

  const handleUpload = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!file) return;

    const trimmedGameName = gameName.trim().replace(/\s+/g, " ");
    if (!trimmedGameName) {
      setError("Game name is required.");
      return;
    }
    if (trimmedGameName.length > 120) {
      setError("Game name must be 120 characters or fewer.");
      return;
    }

    setIsUploading(true);
    setError(null);

    try {
      const supabase = getSupabaseBrowserClient();
      const { data: { session } } = await supabase.auth.getSession();

      if (!session?.access_token) {
        throw new Error("Authentication token is missing. Please sign in again.");
      }
      await syncAuthCookie(session);

      const formData = new FormData();
      formData.append("file", file);
      formData.append("game_name", trimmedGameName);

      try {
        await axios.post("/api/upload-file", formData, {
          headers: {
            "Content-Type": "multipart/form-data"
          }
        });
      } catch (err: unknown) {
        const status = axios.isAxiosError(err) ? err.response?.status : undefined;
        if (status === 413) {
          throw new Error("File exceeds server payload limits.");
        }
        const responseData = axios.isAxiosError(err) ? err.response?.data : undefined;
        const errorMessage = typeof responseData === "object" && responseData !== null && "error" in responseData
          ? String(responseData.error)
          : "";
        throw new Error(errorMessage || `Upload failed with status code ${status || 'network error'}`);
      }

      setUploadSuccess(true);
      setFile(null);
      setGameName("");
      if (onUploadSuccess) {
        onUploadSuccess();
      }
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

  const defaultGameNameFromFile = (selectedFile: File) => {
    return selectedFile.name
      .replace(/\.[^/.]+$/, "")
      .replace(/[_-]+/g, " ")
      .replace(/\s+/g, " ")
      .trim();
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title="Upload Document">
      <div className="flex flex-col w-full gap-4 text-[#4a1900] select-none font-sans py-2">
        
        {uploadSuccess ? (
          <div className="flex flex-col items-center justify-center gap-4 py-6 text-center">
            <div className="flex items-center gap-2 text-2xl text-emerald-700 font-bold drop-shadow-[0_1px_0_rgba(255,255,255,0.4)]">
              <Sparkles className="w-6 h-6 text-emerald-600 animate-pulse" /> Document Uploaded!
            </div>
            <p className="text-[#4a1900] text-md max-w-sm leading-relaxed font-semibold text-center">
              The document is being processed and you may check the status of the document in the dashboard page.
            </p>
            <ButtonGreen 
              className="h-10 w-56 text-lg font-bold text-white mt-2 drop-shadow-[0_1.5px_0_rgba(0,0,0,0.3)]"
              onClick={() => {
                onClose();
                setTimeout(() => {
                  setUploadSuccess(false);
                }, 300);
              }}
            >
              Return to Dashboard
            </ButtonGreen>
          </div>
        ) : (
          <form onSubmit={handleUpload} className="flex flex-col w-full gap-4">
            <RetroInput
              label="Game name"
              id="game-name-input"
              type="text"
              placeholder="e.g. Linear Algebra TD"
              value={gameName}
              onChange={(e) => {
                setGameName(e.target.value);
                setError(null);
              }}
              maxLength={120}
              disabled={isUploading}
            />
            
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
                <FileText className="w-12 h-12 text-[#4a1900]/60 mb-1 animate-bounce" />
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
              <div className="flex items-center justify-between bg-[#3a1d0f]/15 border-2 border-[#4a1900]/25 rounded-xl p-3 w-full animate-fadeIn">
                <div className="flex flex-col items-start gap-0.5 max-w-[75%]">
                  <span className="font-bold text-sm truncate w-full text-[#3a1d0f]">
                    {file.name}
                  </span>
                  <span className="text-xs font-semibold text-[#3a1d0f]/75">
                    {formatFileSize(file.size)}
                  </span>
                </div>
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation();
                    setFile(null);
                    setGameName("");
                  }}
                  className="text-red-700 hover:text-red-900 font-bold text-sm cursor-pointer p-1 active:scale-95 transition-all"
                  disabled={isUploading}
                >
                  Clear
                </button>
              </div>
            )}

            {/* Generation Explanation Warning */}
            <div className="text-xs text-[#3a1d0f]/80 leading-relaxed font-semibold bg-[#3a1d0f]/8 border border-[#4a1900]/20 rounded-lg p-2.5 mt-1 select-none flex items-start gap-1.5 animate-fadeIn">
              <Info className="w-4.5 h-4.5 text-[#3a1d0f]/75 shrink-0 mt-0.5" />
              <p>
                Your study notes will be used to generate the content for your new game and it may take up to 2-3 min depending on the number of pages of your document.
              </p>
            </div>

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
                disabled={isUploading || !file || !gameName.trim()}
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
