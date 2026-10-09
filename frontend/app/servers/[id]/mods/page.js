import { ManagementPage } from "../../../../components/management";
export default async function Page({ params }) {
  const { id } = await params;
  return <ManagementPage id={id} section="mods" />;
}
