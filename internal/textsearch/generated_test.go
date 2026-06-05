package textsearch

import "testing"

func TestIsGeneratedPath(t *testing.T) {
	generated := []string{
		"api/user.pb.go",
		"api/user_grpc.pb.go",
		"proto/user_pb2.py",
		"proto/user_pb2_grpc.py",
		"internal/store/queries.gen.go",
		"internal/store/model_generated.go",
		"internal/svc/client_mock.go",
		"lib/mock_repository.go",
		"gen/zz_generated_deepcopy.go",
		"models/user.g.dart",
		"models/user.freezed.dart",
		"Forms/Main.Designer.cs",
		"types/api.generated.ts",
	}
	for _, p := range generated {
		if !IsGeneratedPath(p) {
			t.Errorf("IsGeneratedPath(%q) = false, want true", p)
		}
	}

	handwritten := []string{
		"internal/store/queries.go",
		"api/user.go",
		"models/user.dart",
		"types/api.ts",
		"parse_string.go", // _string is intentionally NOT a generated marker (FP-prone)
		"cmd/main.go",
		"",
	}
	for _, p := range handwritten {
		if IsGeneratedPath(p) {
			t.Errorf("IsGeneratedPath(%q) = true, want false", p)
		}
	}
}
