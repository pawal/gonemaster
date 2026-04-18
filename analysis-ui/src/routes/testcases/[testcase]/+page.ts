import { getTestcaseDetail, type TestcaseDetail } from "$lib/api";

export type TestcaseDetailPageData = {
  testcase: string;
  module: string;
  datasetTag: string | null;
  detail: TestcaseDetail | null;
  error: string | null;
};

export async function load({ parent, fetch, params, url }): Promise<TestcaseDetailPageData> {
  const layout = await parent();
  const datasetTag = layout.resolvedCohort ?? null;
  const testcase = params.testcase ?? "";
  const module = url.searchParams.get("module") ?? "";

  if (!datasetTag || !testcase || !module) {
    return { testcase, module, datasetTag, detail: null, error: null };
  }

  try {
    const detail = await getTestcaseDetail(module, testcase, { dataset_tag: datasetTag }, fetch);
    return { testcase, module, datasetTag, detail, error: null };
  } catch (error) {
    return {
      testcase,
      module,
      datasetTag,
      detail: null,
      error: error instanceof Error ? error.message : String(error)
    };
  }
}
