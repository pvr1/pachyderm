package server

import (
	"bufio"
	"bytes"
	"io"
	"time"

	"github.com/gogo/protobuf/types"
	"github.com/pachyderm/pachyderm/src/client/pfs"
	"github.com/pachyderm/pachyderm/src/client/pkg/errors"
	"github.com/pachyderm/pachyderm/src/client/pkg/grpcutil"
	"github.com/pachyderm/pachyderm/src/server/pkg/storage/fileset"
	"github.com/pachyderm/pachyderm/src/server/pkg/storage/metrics"
)

func (a *apiServer) FileOperationV2(server pfs.API_FileOperationV2Server) (retErr error) {
	req, err := server.Recv()
	func() { a.Log(req, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(req, nil, retErr, time.Since(start)) }(time.Now())
	return metrics.ReportRequestWithThroughput(func() (int64, error) {
		if err != nil {
			return 0, err
		}
		if !a.env.NewStorageLayer {
			return 0, errors.Errorf("new storage layer disabled")
		}
		repo := req.Commit.Repo.Name
		commit := req.Commit.ID
		var bytesRead int64
		if err := a.driver.withFileSet(server.Context(), repo, commit, func(fs *fileset.FileSet) error {
			for {
				req, err := server.Recv()
				if err != nil {
					if err == io.EOF {
						return nil
					}
					return err
				}
				// TODO Validation.
				switch op := req.Operation.(type) {
				case *pfs.FileOperationRequestV2_PutTar:
					n, err := putTar(fs, server, op.PutTar)
					bytesRead += n
					if err != nil {
						return err
					}
				case *pfs.FileOperationRequestV2_DeleteFiles:
					if err := deleteFiles(fs, op.DeleteFiles); err != nil {
						return err
					}
				}
			}
		}); err != nil {
			return bytesRead, err
		}
		return bytesRead, server.SendAndClose(&types.Empty{})
	})
}

func putTar(fs *fileset.FileSet, server pfs.API_FileOperationV2Server, req *pfs.PutTarRequestV2) (int64, error) {
	ptr := &putTarReader{
		server: server,
		r:      bytes.NewReader(req.Data),
	}
	err := fs.Put(ptr, req.Tag)
	return ptr.bytesRead, err
}

type putTarReader struct {
	server    pfs.API_FileOperationV2Server
	r         *bytes.Reader
	bytesRead int64
}

func (ptr *putTarReader) Read(data []byte) (int, error) {
	if ptr.r.Len() == 0 {
		req, err := ptr.server.Recv()
		if err != nil {
			return 0, err
		}
		op := req.Operation.(*pfs.FileOperationRequestV2_PutTar)
		putTarReq := op.PutTar
		if putTarReq.EOF {
			return 0, io.EOF
		}
		ptr.r = bytes.NewReader(putTarReq.Data)
	}
	n, err := ptr.r.Read(data)
	ptr.bytesRead += int64(n)
	return n, err
}

func deleteFiles(fs *fileset.FileSet, req *pfs.DeleteFilesRequestV2) error {
	for _, file := range req.Files {
		fs.Delete(file, req.Tag)
	}
	return nil
}

func (a *apiServer) GetTarV2(request *pfs.GetTarRequestV2, server pfs.API_GetTarV2Server) (retErr error) {
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, nil, retErr, time.Since(start)) }(time.Now())
	return metrics.ReportRequestWithThroughput(func() (int64, error) {
		if !a.env.NewStorageLayer {
			return 0, errors.Errorf("new storage layer disabled")
		}
		repo := request.File.Commit.Repo.Name
		commit := request.File.Commit.ID
		glob := request.File.Path
		gtw := newGetTarWriter(grpcutil.NewStreamingBytesWriter(server))
		err := a.driver.getTar(server.Context(), repo, commit, glob, gtw)
		return gtw.bytesWritten, err
	})
}

type getTarWriter struct {
	w            io.Writer
	bytesWritten int64
}

func newGetTarWriter(w io.Writer) *getTarWriter {
	return &getTarWriter{w: w}
}

func (gtw *getTarWriter) Write(data []byte) (int, error) {
	n, err := gtw.w.Write(data)
	gtw.bytesWritten += int64(n)
	return n, err
}

func (a *apiServer) GetTarConditionalV2(server pfs.API_GetTarConditionalV2Server) (retErr error) {
	request, err := server.Recv()
	func() { a.Log(request, nil, nil, 0) }()
	defer func(start time.Time) { a.Log(request, nil, retErr, time.Since(start)) }(time.Now())
	return metrics.ReportRequestWithThroughput(func() (int64, error) {
		if err != nil {
			return 0, err
		}
		if !a.env.NewStorageLayer {
			return 0, errors.Errorf("new storage layer disabled")
		}
		repo := request.File.Commit.Repo.Name
		commit := request.File.Commit.ID
		glob := request.File.Path
		var bytesWritten int64
		err := a.driver.getTarConditional(server.Context(), repo, commit, glob, func(fr *FileReader) error {
			if err := server.Send(&pfs.GetTarConditionalResponseV2{FileInfo: fr.Info()}); err != nil {
				return err
			}
			req, err := server.Recv()
			if err != nil {
				return err
			}
			// Skip to the next file (client does not want the file content).
			if req.Skip {
				return nil
			}
			gtcw := newGetTarConditionalWriter(server)
			defer func() {
				bytesWritten += gtcw.bytesWritten
			}()
			w := bufio.NewWriterSize(gtcw, grpcutil.MaxMsgSize)
			if err := fr.Get(w); err != nil {
				return err
			}
			if err := w.Flush(); err != nil {
				return err
			}
			return server.Send(&pfs.GetTarConditionalResponseV2{EOF: true})
		})
		return bytesWritten, err
	})
}

type getTarConditionalWriter struct {
	server       pfs.API_GetTarConditionalV2Server
	bytesWritten int64
}

func newGetTarConditionalWriter(server pfs.API_GetTarConditionalV2Server) *getTarConditionalWriter {
	return &getTarConditionalWriter{
		server: server,
	}
}

func (w *getTarConditionalWriter) Write(data []byte) (int, error) {
	if err := w.server.Send(&pfs.GetTarConditionalResponseV2{Data: data}); err != nil {
		return 0, err
	}
	w.bytesWritten += int64(len(data))
	return len(data), nil
}

func (a *apiServer) ListFileV2(req *pfs.ListFileRequest, server pfs.API_ListFileV2Server) error {
	pachClient := a.env.GetPachClient(server.Context())
	return a.driver.listFileV2(pachClient, req.File, req.Full, req.History, func(finfo *pfs.FileInfoV2) error {
		return server.Send(finfo)
	})
}