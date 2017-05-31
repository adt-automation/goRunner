<%
try
{
out.println("<table border=1><tr><td>Total Memory " + Runtime.getRuntime
().totalMemory()/1000 +" KB</td>");
out.println("<td>Memory in Use "+((Runtime.getRuntime().totalMemory()-
Runtime.getRuntime().freeMemory())/1000) +" KB</td>");
out.println("<td>Thread Count " + Thread.activeCount() + "</td></tr></table>");
}
catch(Exception ex)
{
   out.println(ex);
}
%>
